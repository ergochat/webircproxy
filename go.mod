module github.com/ergochat/webircproxy

go 1.17

require (
	github.com/ergochat/ergo v1.2.1-0.20210919081820-20d8d269ca18
	github.com/ergochat/irc-go v0.0.0-20210617222258-256f1601d3ce
	github.com/gogs/chardet v0.0.0-20191104214054-4b6791f73a28
	github.com/gorilla/websocket v1.4.2
	github.com/okzk/sdnotify v0.0.0-20180710141335-d9becc38acbd
	golang.org/x/text v0.3.7
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/gorilla/websocket => github.com/ergochat/websocket v1.4.2-oragono1

replace github.com/xdg-go/scram => github.com/ergochat/scram v1.0.2-ergo1
