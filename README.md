webircproxy
===========

`webircproxy` is a reverse proxy that accepts [IRCv3-over-WebSocket](https://ircv3.net/specs/extensions/websocket) connections, then forwards them to a conventional ircd that speaks the [normal IRC client-to-server protocol](https://modern.ircdocs.horse/).

This is similar to [webircgateway](https://github.com/kiwiirc/webircgateway), with the following differences:

1. webircproxy implements the [official IRCv3 WebSocket specification](https://ircv3.net/specs/extensions/websocket)
2. For clients using text (i.e., UTF-8) frames, webircproxy implements transcoding from other encodings to UTF-8
3. Consequently, the `ENCODING` command from webircgateway is not implemented. Clients seeking full control over character encodings should negotiate binary frames.
4. Only WebSockets are supported, not SockJS or other legacy transports
5. A number of webircgateway features (reCAPTCHA, ACME, ident, and DNSBLs) are not supported

webircproxy can run behind another reverse proxy, such as nginx; see the [Ergo testnet configs](https://github.com/ergochat/testnet.ergo.chat/blob/e247d9c9cb0cb5aa73e4b126061a79149356854d/nginx_https.conf#L26-L37) for an example of the relevant nginx configuration. It can also run behind a load balancer that sends the PROXY v1 or v2 header. It will pass the best available client IP address (read either from the `X-Forwarded-For` header, the PROXY protocol header, or the client's apparent originating IP address) to the upstream ircd, using the [WEBIRC command](https://ircv3.net/specs/extensions/webirc).

Quick start
-----------

To build `webircproxy`, install an [up-to-date distribution of the Go language for your OS and architecture](https://golang.org/dl/). Then type `make`; this should build a binary named `webircproxy` located at the root of the project.

To run `webircproxy`, provide it with a single command-line argument, the path to its config file. An example config file is provided as `default.yaml`. (Most of webircproxy's functionality is documented as comments in the example config file.)

Transcoding
-----------

`webircproxy` is also intended to serve as a proof-of-concept for server-side transcoding as a transition mechanism for the [UTF8ONLY IRCv3 specification](https://ircv3.net/specs/extensions/utf8-only). Here are some benchmarks for transcoding individual IRC messages:

```
cpu: Intel(R) Core(TM) i3-2130 CPU @ 3.40GHz
BenchmarkTranscodeWithFixedEncoding-4                  	  272278	      4054 ns/op	    2640 B/op	      14 allocs/op
BenchmarkTranscodeWithFixedEncoding2-4                 	  275428	      4420 ns/op	    1744 B/op	      11 allocs/op
BenchmarkTranscodeWithChardet-4                        	    7078	    161428 ns/op	   18384 B/op	      61 allocs/op
BenchmarkTranscodeWithChardet2-4                       	   10000	    119597 ns/op	   17462 B/op	      59 allocs/op
BenchmarkTranscodeWithUnicodeReplacementCharacter-4    	  223276	      6310 ns/op	    1072 B/op	       4 allocs/op
BenchmarkTranscodeWithUnicodeReplacementCharacter2-4   	  329599	      3787 ns/op	    1072 B/op	       4 allocs/op
BenchmarkUTF8Validate-4                                	 2757282	       436.5 ns/op	       0 B/op	       0 allocs/op
```

To rerun these benchmarks on your own hardware: `make bench`.
