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
		<script src="https://cdnjs.cloudflare.com/ajax/libs/msgpack5/4.2.0/msgpack5.min.js" integrity="sha512-D0GVJIuE4FlQJvwnzUBEQ6cb1f72Tg/4iELPcFZpU/a8QPvX805QUm13NhN1kcDtkbrL8Ji/+uyapjaXTqm00Q==" crossorigin="anonymous"></script>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/xterm/3.14.5/xterm.min.js" integrity="sha512-2PRgAav8Os8vLcOAh1gSaDoNLe1fAyq8/G3QSdyjFFD+OqNjLeHE/8q4+S4MEZgPsuo+itHopj+hJvqS8XUQ8A==" crossorigin="anonymous"></script>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/xterm/3.14.5/addons/fit/fit.min.js" integrity="sha512-+wh8VA1djpWk3Dj9/IJDu6Ufi4vVQ0zxLv9Vmfo70AbmYFJm0z3NLnV98vdRKBdPDV4Kwpi7EZdr8mDY9L8JIA==" crossorigin="anonymous"></script>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/xterm/3.14.5/addons/search/search.min.js" integrity="sha512-OkVnWNhmCMHw8pYndhQ+yEMJzD1VrgqF12deRfRcqR6iWL4s8IkxTBwSrJZ2WgpevhD71S68dAqBPHv/VHGDAw==" crossorigin="anonymous"></script>
		<script>
			Terminal.applyAddon(fit)
			Terminal.applyAddon(search)
			var term = new Terminal({
				cursorBlink: 'block',
				macOptionIsMeta: true,
				scrollback: 100
			});
			term.open(document.getElementById('terminal'));
			term.fit();
			term.resize(80, 40);
			//
			var jwt = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibWVuZGVyLnVzZXIiOnRydWUsIm1lbmRlci5wbGFuIjoiZW50ZXJwcmlzZSIsIm1lbmRlci50ZW5hbnQiOiJhYmNkIn0.sn10_eTex-otOTJ7WCp_7NUwiz9lBT0KiPOdZF9Jt4w";
			var socket = new WebSocket("ws://" + window.location.host + "/api/management/v1/deviceconnect/devices/1234567890/connect?jwt=" + jwt);
			socket.onopen = function(e) {
				console.log("[websocket] Connection established");
			};
			socket.onmessage = function(event) {
				event.data.arrayBuffer().then(function (data) {
					obj = msgpack5().decode(data);
					console.log("recv", obj);
					if (obj.cmd == "shell") {
						myString = "";
						for (var i=0; i < obj.data.byteLength; i++) {
							myString += String.fromCharCode(obj.data[i]);
						}
						term.write(myString);
					}
				});
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
			term.on('data', function (data) {
				msg = {cmd: "shell", data: data};
				console.log("send", msg);
				encodedData = msgpack5().encode(msg);
				socket.send(encodedData);
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
