# StaticMD

This is a simple webserver that, when receiving a request, reads an accompanying file, inserts its contents in a template and sends it to the client. 

For content files, they should be found in the `./content` directory inside the root path specified.

If the URL path finds a file which ends in `.md`, then that file is parsed and served with the layout template. If the URL explicitly ends in `.md`, then the raw markdown file is sent.

If the URL path finds a file which ends in `.body`, then that file's contents are passed to the layout template.

Static files in the `./static` directory are served by a simple http.FileServer, don't forget to add index.html to hide contents if need be! 
