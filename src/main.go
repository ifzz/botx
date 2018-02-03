package main

import (
	api "./api"
	"./api/okcoin"
	"./api/zb"
	"errors"
	. "fmt"
	"net/http"
	"time"
	"math/rand"

	"os"
	"os/signal"
	"syscall"
	"io/ioutil"
	"encoding/xml"
	"flag"
	"strings"

)

var systemExit bool = false

type Configure struct {
	XMLName  xml.Name `xml:"config"`
	System string `xml:"system"`
	Exchanges SExchanges `xml:"exchanges""`
}
type SExchanges struct {
	Exchange []SExchange `xml:"exchange"`
}
type SExchange struct {
	Enable bool `xml:"enable"`
	Name string `xml:"name"`
	ApiKey string `xml:"apiKey"`
	SecretKey string `xml:"secretKey"`
	RoiRate float64 `xml:"roiRate`
	BuyLimitMoney float64 `xml:"buyLimitMoney"`
	Coins SCoins `xml:"coins"`
	MaxBotNum int `xml:"maxBotNum"`
}
type SCoins struct {
	Coin []SCoin `xml:"coin"`
}

type SCoin struct{
	Enable bool `xml:"enable"`
	Name string `xml:"name"`
	Pair string `xml:"pair"`
	PriceDecimel string 	`xml:"priceDecimel"`
	AmountDecimel string 	`xml:"amountDecimel"`
}

type Bot struct {
	ID           int              // BotID
	Amount       float64          //当前账户使用金额
	Price        float64          //当前价格
	Type         int              //类型：0，1分别代表买入，卖出
	Timestamp    time.Time            //当前时间
	Status       int              //当前状态：0，1，2分别代表
	RoiRate      float64          //收益率
	Counter      int              //完成个数
	CurrencyPair api.CurrencyPair //
	Exchange     api.API          //
	/*damping = 1/(avr_time + count + 1)*/
	Damping float32 //阻尼系数，表示该bot运行健康度，成交对间隔越短、次数越多，系数越低

	//不同交易所，不同交易货币，精度不一样
	PriceDecimel string //价格精度
	AmountDecimel string //数量精度

	Name string
	StartTime time.Time //启动时间
}

func BuyIn(amount float64, latestOrder *api.Order, bot *Bot) (*api.Order, error) {
	retErr := errors.New(TimeNow() + "挂买单失败")

	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT] 获取Ticker出错，msg: %s\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		return nil, retErr
	}
	//Printf("买入价格: %.4f\n", ticker.Buy)

	buyPrice := ticker.Buy
	buyPrice += 0.01
	buyAmount := amount / buyPrice
	strbuyAmount := Sprintf(bot.AmountDecimel, buyAmount)
	strBuyPrice := Sprintf(bot.PriceDecimel, buyPrice)
	//xx,_:=strconv.ParseFloat(strBuyPrice, 32)
	//Printf("%.4f\n", xx)

	order, err := bot.Exchange.LimitBuy(strbuyAmount, strBuyPrice, bot.CurrencyPair)
	if nil == err {
		Printf("[%s] [%s %s-USDT] 挂买入单 ok : %d, 价格：%s /%s \n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, order.OrderID, strBuyPrice, strbuyAmount)
		return order, nil

	} else {
		Printf("[%s] [%s %s-USDT] 挂买入单 err:%s, 价格：%s /%s \n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, err.Error(),strBuyPrice, strbuyAmount)
	}
	return order, retErr
}

func SellOut(latestOrder *api.Order, bot *Bot , speed int64, roiCfgRate float64) (*api.Order, error) {

	retErr := errors.New("挂卖单失败")
	strSellAmount := Sprintf(bot.AmountDecimel, latestOrder.Amount)

	roiRate:=roiCfgRate
	//如果5分钟内成交，可以增大收益率
	if speed < 60 {//1分钟
		roiRate = roiCfgRate * 5
	}else if speed < 120 {//如果2分钟内成交，可以增大收益率
		roiRate = roiCfgRate * 3
	}else if speed < 300 {//5分钟
		roiRate = roiCfgRate * 2
	}else if speed < 600 { //10分钟
		roiRate = roiCfgRate * 1.5
	}else {
		roiRate = roiCfgRate
	}
	sellPrice := latestOrder.Price * (1 + roiRate)

	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT] 挂卖单时获取Ticker出错，message: %s\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		return nil,retErr
	}
	if ticker.Sell > sellPrice { //如果收益计算后比当前市场卖价格低，直接挂市场卖价
		sellPrice = ticker.Sell
	}
	sellPrice -= 0.01

	strSellPrice := Sprintf(bot.PriceDecimel, sellPrice)

	order, err := bot.Exchange.LimitSell(strSellAmount, strSellPrice, bot.CurrencyPair)
	if nil == err {
		Printf("[%s] [%s %s-USDT] 挂卖出单 ok : %d，价格:%s / %s \n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, order.OrderID, strSellPrice, strSellAmount)
		//put sell order
		return order, nil
	} else {
		Printf("[%s] [%s %s-USDT] 挂卖出单err:%s, 价格：%s /%s \n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error(), strSellPrice, strSellAmount)
	}
	return order, retErr
}

func tryCancelOrder(latestOrder *api.Order, bot *Bot) (bool, error) {

	shouldCancel := false
	retErr := errors.New("挂取消单失败")
	orderID := latestOrder.OrderID2

	ticker, err := bot.Exchange.GetTicker(bot.CurrencyPair)
	if err != nil {
		Printf("[%s] [%s %s-USDT] 获取Ticker出错，message: %s\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		return shouldCancel, retErr
	}

	if (ticker.Buy / bot.Price) > 0.02 {
		//超过2%，可以取消订单
		Printf("[%s] [%s %s-USDT] 取消订单，买入价格：%.4f, 现价: %.4f\n",
			TimeNow(), bot.Exchange.GetExchangeName(), bot.Name, bot.Price, ticker.Buy)
		shouldCancel = true
		_, err := bot.Exchange.CancelOrder(orderID, bot.CurrencyPair)
		if nil == err {
			//成功
			return shouldCancel, nil
		} else {
			Printf("[%s] [%s %s-USDT] 取消订单失败, err:%s\n",
				TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, err.Error())
		}
	}

	return shouldCancel, retErr

}

func TimeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func Start(bot *Bot, exchangeCfg SExchange) {

	Printf("[%s] [%s %s-USDT] start a bot %d \n",
		TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, bot.ID)

	var orderID string = ""

	var speed int64 =1000000 ////speed 挂卖出单，到卖出单交易成功的时间间隔

	for systemExit == false{

		time.Sleep(10 * time.Second)
		acct, err := bot.Exchange.GetAccount()
		if err != nil {
			Printf("[%s] [%s %s-USDT] bot :%d 获取账户出错，继续， 信息：%s\n",
				TimeNow(), bot.Exchange.GetExchangeName(),bot.Name, bot.ID, err.Error())
			continue
		}

		currentAct := acct.SubAccounts[api.USDT].Amount

		//检查订单状态
		if orderID != "" {
			latestOrder, err := bot.Exchange.GetOneOrder(orderID, bot.CurrencyPair)
			if err != nil {
				Printf("[%s] [%s %s-USDT] 读取订单(%s)状态失败：%s\n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,orderID, err.Error())
				continue
			}
			/******DUEBUG**
			Printf("orderID:%d (price:%.4f, amount:%.4f, fee:%.4f, status:%d)\n",
				latestOrder.OrderID, latestOrder.Price,latestOrder.Amount,
					latestOrder.Fee, latestOrder.Status)
			*/

			if latestOrder.Status == api.ORDER_FINISH && latestOrder.Side == api.BUY {
				//订单完成，如果是买入订单，则可以挂卖单
				//Println(TimeNow() + "订单完成，如果是买入订单，则可以挂卖单")
				currentOrder, cerr := SellOut(latestOrder, bot, speed, exchangeCfg.RoiRate)
				if cerr == nil {
					Printf("[%s] [%s %s-USDT] 挂单（卖）订单号： %d\n",
						TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentOrder.OrderID)
					orderID = currentOrder.OrderID2//Sprintf("%d", currentOrder.OrderID) //保存最新ID

					//完成时间
					bot.Timestamp = time.Now()

					//TODO TEST 交易完成一次，则退出
					//break
				}

			} else if latestOrder.Status == api.ORDER_FINISH && latestOrder.Side == api.SELL {

				//订单完成，如果是卖出订单，可以挂买单,
				//speed 挂卖出单，到卖出单交易成功的时间间隔
				speed = time.Now().Unix() - bot.Timestamp.Unix()

				//TODO,仓位管理，如果小于80%仓位，不要买入，不能满仓
				//TODO,对于完成很快的bot，适当调整增加买入量
				if currentAct < bot.Amount  {
					//Printf("[%s][%s %s-USDT]  账户余额不足 :%.4f\n",
					//	TimeNow(),bot.Exchange.GetExchangeName(),bot.Name, currentAct)
					continue
				}

				//Println(TimeNow() + "订单完成，如果是卖出订单，可以挂买单")

				currentOrder, cerr := BuyIn(bot.Amount, latestOrder, bot)
				if cerr == nil {
					Printf("[%s] [%s %s-USDT] 挂单（买）成功，订单号：%d\n",
						TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentOrder.OrderID)
					orderID = currentOrder.OrderID2//Sprintf("%d", currentOrder.OrderID) //保存最新ID

					//统计当前收益率
					bot.Counter++
					bot.RoiRate += latestOrder.Price / currentOrder.Price //ROI_RATE
					bot.Damping = bot.Damping * 0.9
					//bot.Damping = bot.Damping + float64(time.Now().Unix() - bot.Timestamp) / float64(60 * 60 * 1000)

					//完成时间
					bot.Timestamp = time.Now()
				}

			} else if latestOrder.Status == api.ORDER_CANCEL {
				//取消订单了
				Printf("[%s] [%s %s-USDT] 订单号：%s 被取消 \n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,orderID)
				orderID = ""
			} else {

				//如果长时间(1小时)未成交，且为买入单，尝试取消订单
				if latestOrder.Side == api.BUY &&  (time.Now().Unix() - bot.Timestamp.Unix()) > 3600 {
					shouldCancel, cerr := tryCancelOrder(latestOrder, bot)
					if cerr == nil && shouldCancel == true {
						//需要取消订单，且已经成功
						Printf("[%s] [%s %s-USDT] 取消订单:%s 成功\n",
							TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,orderID)
						orderID = ""

					}
				}
			}

		} else {
			//第一次进入，直接尝试买入

			if currentAct < bot.Amount {
				//Printf("[%s]  [%s %s-USDT]账户余额不足 :%.4f\n",
				//	TimeNow(), bot.Exchange.GetExchangeName(),bot.Name,currentAct)
				continue
			}
			Printf("[%s] [%s %s-USDT]第一次进入，直接尝试买入\n",
				TimeNow(),bot.Exchange.GetExchangeName(),bot.Name)

			var orderTmp *api.Order
			currentOrder, cerr := BuyIn(bot.Amount, orderTmp, bot)
			if cerr == nil {
				Printf("[%s] [%s %s-USDT] 挂单（买）成功, 订单号：%s, %d\n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name, currentOrder.OrderID2, currentOrder.OrderID)
				orderID = currentOrder.OrderID2//Sprintf("%d", currentOrder.OrderID) //保存最新ID

				//完成时间
				bot.Timestamp = time.Now()

			} else {
				Printf("[%s] [%s %s-USDT] 第一次进入，买入失败\n",
					TimeNow(), bot.Exchange.GetExchangeName(),bot.Name)
			}
		}

	}

	Printf("[%s] [%s %s-USDT] bot完成认为，结束\n",
		TimeNow(), bot.Exchange.GetExchangeName(),bot.Name)

}

func roiCalculate(bots [10000]Bot, cnt int) (bool, int) {
	roiRate := 0.0
	counter := 0
	roiWell:=true//加速度，确定等待时间，是否有收益，决定是否可以启动新的bot

	//很长时间未成交，可以新增
	//成交很快，可以新增

	var timeSpan int64 = 0
	for i := 0; i < cnt; i++ {
		roiRate += bots[i].RoiRate
		counter += bots[i].Counter
		timeSpan += (time.Now().Unix() - bots[i].Timestamp.Unix())

	}

	av := float64(timeSpan) / float64(cnt)
	if roiRate >=0.0  {
		//整体有收益的情况，且平均成单时间小于10分钟，或大于60分钟，经验值
		if av < 600 || av > 3600 {
			roiWell = true
		}
	}
	return roiWell, counter
}
func startBots(bot Bot, exchangeCfg SExchange)  {
	//bot start，启动策略
	maxCnt := exchangeCfg.MaxBotNum
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var bots [10000]Bot //最大启动10000个机器人

	if maxCnt > 10000 {
		maxCnt = 10000
	}

	currBotID := 0

	currCnt:=0

	for systemExit == false{

		//满足一定条件，启动一个新的bot

		//计算收益率情况, roi
		roiWell, counter:=roiCalculate(bots, currBotID)

		//只要有收益，就可以启动新的bot
		if roiWell==true && maxCnt > currCnt {
			bots[currBotID] = bot //初始化
			bots[currBotID].ID = currBotID + 1 //修改ID
			bots[currBotID].StartTime = time.Now() //启动时间
			go Start(&bots[currBotID], exchangeCfg)
			currCnt++
		}

		//设置间隔，10分钟
		span := time.Duration( 10 * time.Minute + time.Duration(r.Intn(1000000)))
		time.Sleep(span)


		Printf("[%s] [%s %s-USDT] 累积成交对：%d\n",
			TimeNow(),bot.Exchange.GetExchangeName(), bot.Name, counter)
	}

}

func startExchange(exchange api.API, exchangeCfg SExchange)  {

	Printf("[%s] 启动%s bot\n",TimeNow(),exchange.GetExchangeName() )
	balanceBegin := getBalance(exchange, nil)

	oldTime:=time.Unix(1480390585, 0)
	for _, coin := range exchangeCfg.Coins.Coin {
		if coin.Enable == false {
			continue
		}
		Printf("[%s] %s - [%s-USDT] 启动状态 \n",TimeNow(),exchange.GetExchangeName(),coin.Name )
		pair:= api.CurrencyPair{api.Currency{coin.Name,""}, api.USDT}
		coinBot :=  Bot{0, 1, 0.0,
			0, oldTime, 0, 0.0, 0,
			pair, exchange, 1.0,
			coin.PriceDecimel, coin.AmountDecimel, coin.Name, time.Now()} //初始化
		go startBots(coinBot, exchangeCfg)
		time.Sleep(time.Second)
	}

	for systemExit == false { //主线程等待

		//获取盈利情况//计算收益
		balanceNow := getBalance(exchange, nil)
		rate:= (api.ToFloat64(balanceNow) - api.ToFloat64(balanceBegin)) / api.ToFloat64(balanceBegin)
		rate = rate * 100
		Printf("[%s] [%s-USDT] 开始余额：%s, 当前余额: %s，整体累积收益率：%.4f %%\n",
			TimeNow(),exchange.GetExchangeName(), balanceBegin, balanceNow, rate)

		time.Sleep(10 * time.Minute)

	}

}
/*
get Balance
 */
func getBalance(exchange api.API, currency *api.Currency) string {
	acc, err := exchange.GetAccount()
	if err != nil {
		//error
		Printf("[%s] getBalance error , err message: %s\n", TimeNow(), err.Error())
		return "0.0000"
	}
	balance := 0.0
	for curr, subItem := range acc.SubAccounts {
		if currency != nil {

			if curr == *currency {
				amount := subItem.Amount + subItem.ForzenAmount

				if amount > 0 {
					ticker, err := exchange.GetTicker(api.CurrencyPair{*currency, api.USDT})
					if err != nil {
						Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
							TimeNow(), curr.String(), err.Error())
						return "0.0000"
					}
					balance += amount * ticker.Last
				}

				break
			}
		}else {//get full
			amount := subItem.Amount + subItem.ForzenAmount

			if amount > 0 {
				if curr != api.USDT {
					ticker, err := exchange.GetTicker(api.CurrencyPair{curr, api.USDT})
					if err != nil {
						Printf("[%s] getBalance of %s error of get ticker , err message: %s\n",
							TimeNow(), curr.String(), err.Error())
						continue //忽略掉
					}
					//Printf("%s,%.4f\n",curr.String(), amount)
					balance += amount * ticker.Last
				} else {
					balance += amount
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return Sprintf("%.4f", balance)

}

func ExitFunc()  {

	systemExit = true

	time.Sleep(10 * time.Second)

	os.Exit(0)
}

func loadConfigure(filePath string) (Configure, error) {

	var cfg Configure
	file, err := os.Open(filePath) // For read access.
	if err != nil {
		Printf("error: %v", err)
		return cfg, errors.New("open config file failed")
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		Printf("error: %v", err)
		return cfg, errors.New("read file failed")
	}

	err = xml.Unmarshal(data, &cfg)
	if err != nil {
		Printf("error: %v", err)
		return cfg, errors.New("xml parse failed")
	}
	return cfg,nil
}

func main() {

	//Println(filepath.Dir(os.Args[0]))

	configFile := flag.String("conf", "../conf/config.xml", "load config file")

	config ,err:= loadConfigure(*configFile)

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
				Println("系统退出", s)
				ExitFunc()
			case syscall.SIGUSR1:
				Println("usr1", s)
			case syscall.SIGUSR2:
				Println("usr2", s)
			default:
				Println("other", s)
			}
			time.Sleep(time.Second)
		}
	}()

	for _,v:= range config.Exchanges.Exchange {
		Println("xxx")
		var exchange api.API
		switch strings.ToUpper(v.Name) {
		case "ZB":
			Printf("[%s] zb\n", TimeNow())
			exchange = zb.New(http.DefaultClient,
				v.ApiKey, v.SecretKey)
			break;
		case "OKEX":
			Printf("[%s] ok\n", TimeNow())
			exchange = okcoin.NewOKExSpot(http.DefaultClient, v.ApiKey, v.SecretKey)
			break
		default:
			break
		}

		if v.Enable == true {

			go startExchange(exchange, v)
		} else {
			Printf("[%s] %s not enable\n", TimeNow(), v.Name)
		}
	}


	for systemExit == false { //主线程等待

		time.Sleep(time.Second)

	}

	Printf("[%s] 系统正在退出\n", TimeNow())

}
