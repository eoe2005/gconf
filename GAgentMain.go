package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)
//配置信息
var ConfigMap map[string]GNode = make(map[string]GNode)
var stopHeart chan struct{} = make(chan struct{})
var ReConnectServer chan struct{} = make(chan struct{})

type GNode struct {
	Value string
	GList map[string]GNode
}

//noinspection GoVarAndConstDeclaration
func AddConfig(conf map[string]GNode,keys []string,value string)  {
	if len(keys) == 0 {
		return
	}
	fmt.Printf("添加内容 %d\n",len(keys))
	k := string(keys[0])
	var gnode GNode
	if node, ok := conf[k]; ok {
		gnode = node
	}else{
		if len(keys) == 1{
			gnode = GNode{value,make(map[string]GNode)}
			conf[k] = gnode

			d,_:=json.Marshal(ConfigMap)
			println(string(d))
			return
		}else{
			gnode = GNode{"",make(map[string]GNode)}
			conf[k] = gnode
		}

		println("查找到的内容",gnode.Value)
	}
	AddConfig(gnode.GList,keys[1:],value)

}
func DeleteConfig(conf map[string]GNode,keys []string)  {
	k := string(keys[0])
	var gnode GNode
	if node, ok := conf[k]; ok {
		if len(keys) == 1{
			delete(conf,k)

			d,_:=json.Marshal(ConfigMap)
			println(string(d))
			return
		}else{
			gnode = node
		}

	}else{
		return
	}
	DeleteConfig(gnode.GList,keys[1:])

}
//查找配置
func FindValByKey(keys []byte) string{
	keysSplit := strings.Split(string(keys),"/")
	println(string(keys),keysSplit[0])
	var valMap = ConfigMap
	var gnode GNode
	for index := range keysSplit{
		println(string(keysSplit[index]))
		//noinspection GoUnresolvedReference
		if node, ok := valMap[string(keysSplit[index])]; ok {
			println(node.Value)
			gnode = node
			valMap = node.GList
		}else{
			return ""
		}

	}
	return gnode.Value
}
//获取配置
func ClientGet(con net.Conn){
	defer con.Close()
	var keys []byte = make([]byte,1024)
	//println(keys)
	len,err := con.Read(keys)
	//println(string(keys[:len]))
	if err != nil{
		con.Write([]byte("\r\n"))
	}
	con.Write([]byte(FindValByKey(keys[:len])+"\r\n"))
}

//定期向服务器发送心跳包
func ServerHeart(con *net.TCPConn){
	con.Write([]byte("H:"))
}

func AgentUnix(){
	filename := "/tmp/GConfAgent.sock"
	os.Remove(filename)
	defer os.Remove(filename)
	addr,err := net.ResolveUnixAddr("unix",filename)
	if err != nil{
		fmt.Println("创建socket失败2")
		os.Exit(-1)
	}
	listener ,err:= net.ListenUnix("unix",addr)
	defer listener.Close()
	if err != nil{
		fmt.Println("创建socket失败1",err)
		os.Exit(-1)
	}
	for{
		con,err := listener.Accept()
		if err != nil{
			con.Close()
		}
		go  ClientGet(con)
	}
	//stopHeart<-
}
// 重新链接服务器
func resetServerCon(con *net.TCPConn){
	if con != nil{
		con.Close()
	}

	re := time.After(1100 * time.Second)
	<- re
	ServerCon()
}
func ReadServerConf(con *net.TCPConn){
	println("连接服务器")
	read := bufio.NewReader(con)
	for{
		data,_,err :=read.ReadLine()
		if err != nil{
			if err == io.EOF{
				resetServerCon(con)
				return
			}
		}
		println(string(data))
		strdata := string(data)
		//strdata = strdata[:len(strdata)-2]
		index := 2
		if string(strdata[2:3]) == "/"{
			index = 3
		}
		switch string(strdata[0:2]) {
		case "H:":
			println("心跳包正常")
		case "A:":
			println("新增配置",strdata)
			dd := strings.Split(string(strdata[index:]),":::")
			AddConfig(ConfigMap,strings.Split(dd[0],"/"),dd[1])
		case "E:":
			println("修改配置",strdata)
			dd := strings.Split(string(strdata[index:]),":::")
			DeleteConfig(ConfigMap,strings.Split(dd[0],"/"))
			AddConfig(ConfigMap,strings.Split(dd[0],"/"),dd[1])
		case "D:":
			println("删除配置",strdata)
			DeleteConfig(ConfigMap,strings.Split(string(strdata[index:]),"/"))
		}
	}
}
//连接服务器
func ServerCon(){
	addr,err := net.ResolveTCPAddr("tcp","127.0.0.1:8082")
	if err != nil{
		fmt.Println("链接服务器失败1")
		resetServerCon(nil)
	}
	con,err := net.DialTCP("tcp",nil,addr)
	if err != nil{
		resetServerCon(nil)
		println("服务器链接失败")
	}
	defer resetServerCon(con)
	go ReadServerConf(con)
	t := time.Tick(3 * time.Second)
	for {
		 select{
		 case <-t:
			 ServerHeart(con)
		 	case <-stopHeart:
		 		return
		 }
	}
}
func main() {
	//AddConfig(ConfigMap,[]string{"app","test"},"test001")
	//AddConfig(ConfigMap,[]string{"app","test","aac"},"test00")
	//AddConfig(ConfigMap,[]string{"app","test2"},"test002")


	//d,_:=json.Marshal(ConfigMap)
	//println(string(d))

	//eleteConfig(ConfigMap,[]string{"app","test","aac"})

	//d2,_:=json.Marshal(ConfigMap)
	//println(string(d2))

	go ServerCon()
	AgentUnix()
}
