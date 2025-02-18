package main

import (
	config "config_service/kitex_gen/config/configservice"
	"github.com/cloudwego/kitex/server"
	"log"
	"net"
)

func main() {
	addr, _ := net.ResolveTCPAddr("tcp", ":10001")
	svr := config.NewServer(
		new(ConfigServiceImpl),
		server.WithServiceAddr(addr),
	)

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
