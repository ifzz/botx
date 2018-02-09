package main


import (
	"flag"
	"net/http"
	api "./api"
	"./api/zb"
	"./api/okcoin"
	. "./stratage"
	. "fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"strings"
	"stratage"
)


func main()  {

	configFile := flag.String("c", "../conf/config.xml", "load config file")
	flag.Parse()
	config, err := loadConfigure(*configFile)

	if err != nil {
		Printf("[%s] 加载配置文件失败，系统正在退出\n", TimeNow())
		return
	}
	//创建监听退出chan
	c := make(chan os.Signal)
	//监听指定信号 ctrl+c kill
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				Printf("[%s] 接收到退出信号，即将退出\n", TimeNow())
				ExitFunc()
				break
			default:
				Printf("[%s] 接收到sys信号，即将退出\n", TimeNow())
				break
			}
			time.Sleep(time.Second)
		}
	}()

	for _, v := range config.Exchanges.Exchange {

		var exchange api.API
		switch strings.ToUpper(v.Name) {
		case "ZB":
			Printf("[%s] zb\n", TimeNow())
			exchange = zb.New(http.DefaultClient,
				v.ApiKey, v.SecretKey)
			break
		case "OKEX":
			Printf("[%s] ok\n", TimeNow())
			exchange = okcoin.NewOKExSpot(http.DefaultClient, v.ApiKey, v.SecretKey)
			break
		default:
			break
		}

		if v.Enable == true {

			go Observing(exchange, v)

		} else {
			Printf("[%s] %s not enable\n", TimeNow(), v.Name)
		}
	}

	for systemExit == false { //主线程等待

		time.Sleep(5 * time.Second)

	}

	Printf("[%s] 系统正在退出\n", TimeNow())


}