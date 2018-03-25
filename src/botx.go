package main

import (
	"./api"
	"./api/zb"
	"./stratagy/standard"
	"./stratagy/bigstep"
	"./stratagy/allin"
	"./stratagy/double"
	"./stratagy/single"
	. "./common"
	. "fmt"
	"net/http"
	"time"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var systemStatus int = 0
func exitNormal()  {

	systemStatus = 1
	time.Sleep(10 * time.Second)
	os.Exit(0)
}
//botx入口
func main() {


	configFile := flag.String("c", "../conf/config.xml", "load config file")
	//model := flag.String("m","standard","exchange model (default in standard model,standard/observe/double)")
	flag.Parse()
	config, err := LoadConfigure(*configFile)

	if err != nil {
		Printf("[%s] 加载配置文件失败，系统正在退出\n", TimeNow())
		return
	}
	systemStatus = 0
	//创建监听退出chan
	c := make(chan os.Signal)
	//监听指定信号 ctrl+c kill
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for s := range c {
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				Printf("[%s] 接收到退出信号，即将退出\n", TimeNow())
				exitNormal()
			default:
				Printf("[%s] 接收到sys信号，即将退出\n", TimeNow())
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
			klines,err := exchange.GetKlineRecords(api.BTC_USDT,"1min","1000","")
			if err != nil {
				Printf("err: %s\n", err.Error())
			}else {
				for _,kline := range klines {
					Printf("%.4f %.4f %.4f %.4f %.4f\n", kline.Open, kline.Close, kline.High, kline.Low, kline.Vol)
				}
			}
			return
			break
		case "OKEX":
			Printf("[%s] ok\n", TimeNow())
			//exchange = okcoin.NewOKExSpot(http.DefaultClient, v.ApiKey, v.SecretKey)
			break
		default:
			break
		}

		if v.Enable == true {
			Printf("[%s] %s mode\n", TimeNow(), config.Strategy)
			switch strings.ToUpper(config.Strategy) {
			case "DOUBLE":
				 go double.Start(exchange, v, &systemStatus)
				break;
			case "STANDARD":
				go standard.Start(exchange, v, &systemStatus)
			case "SINGLE":
				go single.Start(exchange, v, &systemStatus)
			case "ALLIN":
				go allin.Start(exchange, v, &systemStatus)
			case "BIGSTEP":
				go bigstep.Start(exchange,v,&systemStatus)
			}

		} else {
			Printf("[%s] %s not enable\n", TimeNow(), v.Name)
		}
	}

	for systemStatus == 0 {
		time.Sleep(5 * time.Second)

	}
	Printf("[%s] 系统正在退出\n", TimeNow())

}
