package jsrunner

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/robertkrimen/otto"
	_ "github.com/robertkrimen/otto/underscore"
)

var funcMap = map[string]func(otto.FunctionCall) otto.Value{
	"gFetch": func(fc otto.FunctionCall) otto.Value {
		if len(fc.ArgumentList) != 1 || !fc.ArgumentList[0].IsString() {
			return fc.Otto.MakeTypeError("Invalid gFetch function arguments.")
		}
		url := fc.ArgumentList[0].String()

		data, err := http.Get(url)
		if err != nil {
			return fc.Otto.MakeCustomError("GetError", err.Error())
		}

		val, err := fc.Otto.ToValue(data)
		if err != nil {
			log.Println(err)
			return otto.NullValue()
		}
		return val
	},
	"retData": func(fc otto.FunctionCall) otto.Value {
		var rez = make(map[string]otto.Value)
		if len(fc.ArgumentList) != 3 {
			return fc.Otto.MakeTypeError("Invalid retData function arguments.")
		}
		rez["errCode"] = fc.ArgumentList[0]
		rez["status"] = fc.ArgumentList[1]
		rez["data"] = fc.ArgumentList[2]
		val, err := fc.Otto.ToValue(rez)
		if err != nil {
			log.Println(err)
			return otto.NullValue()
		}
		return val
	},
}

type CGI struct {
	vm *otto.Otto
	f  otto.Value
}

func (c *CGI) genInputs(r *http.Request) otto.Value {
	var rez = make(map[string]string)
	for k, v := range r.Form {
		if len(v) == 0 {
			rez[k] = ""
		} else {
			rez[k] = v[0]
		}
	}
	for k, v := range r.PostForm {
		if len(v) == 0 {
			rez[k] = ""
		} else {
			rez[k] = v[0]
		}
	}
	val, err := c.vm.ToValue(rez)
	if err != nil {
		panic(err)
	}
	return val
}

func (c *CGI) Execute(w http.ResponseWriter, r *http.Request) error {
	r.ParseForm()
	out, err := c.f.Call(otto.NullValue(), c.genInputs(r))
	if err != nil {
		return err
	}
	obj := out.Object()
	if obj == nil {
		return errors.New("Returned value is not return object.")
	}
	statusVal, err := obj.Get("status")
	if err != nil || !statusVal.IsString() {
		return errors.New("Malformed return object.")
	}

	errCodeVal, err := obj.Get("errCode")
	if err != nil || !errCodeVal.IsNumber() {
		return errors.New("Malformed return object.")
	}

	dataVal, err := obj.Get("data")
	if err != nil || !dataVal.IsDefined() {
		return errors.New("Malformed return object.")
	}

	status := statusVal.String()
	errCode, _ := errCodeVal.ToInteger()
	data, _ := dataVal.Export()

	w.WriteHeader(int(errCode))
	return json.NewEncoder(w).Encode(struct {
		Status string      `json:"status"`
		Data   interface{} `json:"data"`
	}{status, data})
}

func (c *CGI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := c.Execute(w, r); err != nil {
		json.NewEncoder(w).Encode(struct {
			Status string      `json:"status"`
			Data   interface{} `json:"data"`
		}{"error", err.Error()})
	}
}

func NewCGI(source io.Reader) (*CGI, error) {
	vm := otto.New()
	for k, v := range funcMap {
		vm.Set(k, v)
	}

	_, err := vm.Run(source)
	if err != nil {
		return nil, err
	}

	val, err := vm.Get("handle")
	if err != nil {
		return nil, err
	}

	if !val.IsFunction() {
		return nil, errors.New("`handle` not function")
	}

	return &CGI{vm, val}, nil
}
