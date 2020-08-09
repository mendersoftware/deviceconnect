// Copyright 2019 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const template = `<!doctype html>
<html>
  <head>
	<title>Mender Connect</title>
	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/xterm/3.14.5/xterm.min.css" integrity="sha512-iLYuqv+v/P4u9erpk+KM83Ioe/l7SEmr7wB6g+Kg1qmEit8EShDKnKtLHlv2QXUp7GGJhmqDI+1PhJYLTsfb8w==" crossorigin="anonymous" />
  </head>
  <body>
	<div id="terminal"></div>
	<script src="https://cdnjs.cloudflare.com/ajax/libs/xterm/3.14.5/xterm.min.js" integrity="sha512-2PRgAav8Os8vLcOAh1gSaDoNLe1fAyq8/G3QSdyjFFD+OqNjLeHE/8q4+S4MEZgPsuo+itHopj+hJvqS8XUQ8A==" crossorigin="anonymous"></script>
	<script>
	  var term = new Terminal();
	  term.open(document.getElementById('terminal'));
	  term.resize(80, 25);
	  //
	  var socket = new WebSocket("ws://" + window.location.host + "/api/v1/websocket");
	  socket.onopen = function(e) {
		console.log("[websocket] Connection established");
	  };
	  socket.onmessage = function(event) {
		data = JSON.parse(event.data) || {};
		if (data.cmd == "terminal") {
		  term.write(atob(data.data).replace(/\r/g, "\n\r"));
		}
	  };
	  socket.onclose = function(event) {
		if (event.wasClean) {
		  console.log("[close] Connection closed cleanly, code=" + event.code + " reason=" + event.reason);
		} else {
		  console.log('[close] Connection died');
		}
	  };
	  socket.onerror = function(error) {
		console.log("[error] " + error.message);
	  };
	  //
	  term.onData(function (data) {
		term.write(data.replace(/\r/g, "\n\r"));
		msg = {cmd: "terminal", data: btoa(data)};
		socket.send(JSON.stringify(msg));
	  })
	</script>
  </body>
</html>`

// TerminalController contains status-related end-points
type TerminalController struct{}

// NewTerminalController returns a new TerminalController
func NewTerminalController() *TerminalController {
	return new(TerminalController)
}

// Terminal responds to GET /status
func (h TerminalController) Terminal(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(template))
}
