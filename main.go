package main

import (
	"encoding/json"
	"flag"
	"github.com/ahmetb/go-linq/v3"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

var commandModel CommandModel
var configModel ConfigurationModel

func main() {
	initCommandModel()
	loadConfig()

	if commandModel.Interval == nil || *commandModel.Interval == 0 {
		update()
		return
	}

	intervalFunction()
}

func update() {
	//publicIp := getPublicIp()
	publicIp := getLocalIP()
	log.Printf("Local IP: %s...", publicIp)

	subDomains := getSubDomains()
	for _, sub := range subDomains {
		if sub.Value != publicIp {
			// 更新域名绑定的 IP 地址。
			sub.Value = publicIp
			sub.TTL = linq.From(*configModel.SubDomains).FirstWith(func(subDomain interface{}) bool {
				return subDomain.(SubDomainModel).Name == sub.RR
			}).(SubDomainModel).Interval
			updateSubDomain(&sub)
		}
	}

	log.Printf("域名记录更新成功...")
}

func intervalFunction() {
	tick := time.Tick(time.Second * time.Duration(*commandModel.Interval))
	for {
		select {
		case <-tick:
			update()
		}
	}
}

func initCommandModel() {
	commandModel.FilePath = flag.String("f", "", "指定自定义的配置文件，请传入配置文件的路径。")
	commandModel.Interval = flag.Int("i", 0, "指定程序的自动检测周期，单位是秒。")

	flag.Parse()
}

func loadConfig() {
	var configFile string
	if *commandModel.FilePath == "" {
		dir, _ := os.Getwd()
		configFile = path.Join(dir, "settings.json")
	} else {
		configFile = *commandModel.FilePath
	}

	// 打开配置文件，并进行反序列化。
	f, err := os.Open(configFile)
	if err != nil {
		log.Fatalf("无法打开文件：%s", err)
		os.Exit(-1)
	}
	defer f.Close()
	data, _ := ioutil.ReadAll(f)

	if err := json.Unmarshal(data, &configModel); err != nil {
		log.Fatalf("数据反序列化失败：%s", err)
		os.Exit(-1)
	}
}

func getPublicIp() string {
	resp, err := http.Get(GetPublicIpUrl)
	if err != nil {
		log.Printf("获取公网 IP 出现错误，错误信息：%s", err)
		os.Exit(-1)
	}
	defer resp.Body.Close()

	bytes, _ := ioutil.ReadAll(resp.Body)

	return strings.Replace(string(bytes), "\n", "", -1)
}

func getSubDomains() []alidns.Record {
	client, err := alidns.NewClientWithAccessKey("cn-hangzhou", configModel.AccessId, configModel.AccessKey)

	request := alidns.CreateDescribeDomainRecordsRequest()
	request.Scheme = "https"

	request.DomainName = configModel.MainDomain

	response, err := client.DescribeDomainRecords(request)
	if err != nil {
		log.Println(err.Error())
	}

	// 过滤符合条件的子域名信息。
	var queryResult []alidns.Record
	linq.From(response.DomainRecords.Record).Where(func(c interface{}) bool {
		return linq.From(*configModel.SubDomains).Select(func(x interface{}) interface{} {
			return x.(SubDomainModel).Name
		}).Contains(c.(alidns.Record).RR)
	}).ToSlice(&queryResult)

	return queryResult
}

func updateSubDomain(subDomain *alidns.Record) {
	client, err := alidns.NewClientWithAccessKey("cn-hangzhou", configModel.AccessId, configModel.AccessKey)

	request := alidns.CreateUpdateDomainRecordRequest()
	request.Scheme = "https"
	request.RecordId = subDomain.RecordId
	request.RR = subDomain.RR
	request.Type = subDomain.Type
	request.Value = subDomain.Value
	request.TTL = requests.NewInteger64(subDomain.TTL)

	_, err = client.UpdateDomainRecord(request)
	if err != nil {
		log.Print(err.Error())
	}
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	return ""
}
