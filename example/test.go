package main

import (
	logs "github.com/huizluo/logging"
)

func main() {
	logs.SetLogger(logs.FILELOG, `{"filename":"app.log","level":6,"maxlines":0,"maxsize":0,"daily":true,"maxdays":10}`)
	logs.EnableDepth(true)
	//logs.SetLogger("console", "")
	logs.Info("skjadjsnakjd")
}

