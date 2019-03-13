package main

import (
	"bufio"
	"crypto/md5"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/larspensjo/config"
	_ "github.com/mattn/go-sqlite3"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var AgentClientMap map[uintptr]*net.TCPConn = make(map[uintptr]*net.TCPConn)

/********************************* 基础操作 *********************************/

func CheckError(err error){
	if err != nil{
		fmt.Println("系统错误",err)
		os.Exit(-1)
	}
}
// 获取配置字符串
func GetConfString(conf *config.Config ,key ,defval string) string{
	if conf.HasOption("default",key){
		val ,err := conf.String("default",key)
		if err != nil{
			return defval
		}
		return val
	}
	return defval
}
// 获取配置的int
func GetConfInt(conf *config.Config ,key  string,defval int) int{
	if conf.HasOption("default",key){
		val ,err := conf.Int("default",key)
		if err != nil{
			return defval
		}
		return val
	}
	return defval
}


/********************************* 数据库操作 *********************************/
	type DbConf struct {
		Db *sql.DB
		Table string
		Lock *sync.Mutex
	}

	var ConfCon DbConf
	//添加记录
	func (db *DbConf) AddConf(key,val,name string,pid int64) int64 {
		db.Lock.Lock()
		defer db.Lock.Unlock()
		st,err := db.Db.Prepare(fmt.Sprintf("INSERT INTO %s(key,val,pid,add_time,name) VALUES(?,?,?,?,?)",db.Table))
		if err != nil{
			fmt.Print(err)
			return 0
		}
		res,err := st.Exec(key,val,pid,time.Now().Unix(),name)
		defer st.Close()
		if err != nil{
			fmt.Print(err)
			return 0
		}
		rid,err := res.LastInsertId()
		if err != nil{
			fmt.Print(err)
			return 0
		}
		go SendUpdateDataToAllClient(rid,"A","")
		return rid
	}
	//删除记录
	func (db *DbConf) DelConf(id int64) int64  {
		db.Lock.Lock()
		defer db.Lock.Unlock()
		node := ConfCon.FindConfByIdMap(id)
		st,err := db.Db.Prepare(fmt.Sprintf("DELETE FROM %s WHERE id=?",db.Table))
		defer st.Close()
		if err != nil{
			return 0
		}
		res,err := st.Exec(id)
		if err != nil{
			return 0
		}
		rid,err := res.RowsAffected()

		if err != nil{
			return 0
		}

		if node != nil{
			fmt.Printf("删除数据： %v %v\n",node["pid"],node["key"])
			go SendUpdateDataToAllClient(node["pid"].(int64),"D",node["key"].(string))
		}
		return rid
	}

	//修改记录
	func (db *DbConf) Update(id int64,key,val,name string) int64  {
		db.Lock.Lock()
		defer db.Lock.Unlock()
		st,err := db.Db.Prepare(fmt.Sprintf("UPDATE %s SET key=?,val=?,name=? WHERE id=?",db.Table))
		defer st.Close()
		if err != nil{
			fmt.Print(err)
			return 0
		}
		res,err := st.Exec(key,val,name,id)
		if err != nil{
			fmt.Print(err)
			return 0
		}
		rid,err := res.RowsAffected()
		if err != nil{
			fmt.Print(err)

			return 0
		}
		go SendUpdateDataToAllClient(rid,"E","")
		return rid
	}
	/**
	 查询所有的子配置信息
	 */
	func (db *DbConf) GetConfByPid(pid int64) map[int64]map[string]string  {

		st,err := db.Db.Prepare(fmt.Sprintf("SELECT * FROM %s WHERE pid=?",db.Table))
		defer st.Close()
		if err != nil{
			fmt.Println(err)
			return nil
		}
		res,err := st.Query(pid)
		defer res.Close()
		if err != nil{
			fmt.Println(err)
			return nil
		}
		retList := make(map[int64]map[string]string)
		for res.Next(){
			var id int64
			var key string
			var val string
			var name string
			var pid int64
			var addtime int64
			err := res.Scan(&id,&key,&val,&pid,&addtime,&name)
			if err != nil{
				fmt.Println(err)
				return nil
			}
			retList[id]=map[string]string{
				"id" : string(id),
				"key" : key,
				"val" : val,
				"pid" : string(pid),
				"name":name,
				"addtime":string(addtime),
			}
		}
		return retList
	}
	func (db *DbConf) FindById(id int64) (int64,int64,string){
		st,err := db.Db.Prepare(fmt.Sprintf("SELECT * from %s WHERE id=?",db.Table))
		defer st.Close()
		if err != nil{
			fmt.Print(err)
			return 0,0,""
		}
		res,err := st.Query(id)
		defer res.Close()
		if err != nil{
			fmt.Print(err)
			return 0,0,""
		}
		if res.Next(){
			var id int64
			var key string
			var val string
			var name string
			var pid int64
			var addtime int64
			err := res.Scan(&id,&key,&val,&pid,&addtime,&name)
			if err != nil{
				fmt.Print(err)
				return 0,0,""
			}
			return id,pid,key
		}
		return 0,0,""
   }

	func (db *DbConf) FindConfByIdMap(id int64) map[string]interface{}{
		st,err := db.Db.Prepare(fmt.Sprintf("SELECT * from %s WHERE id=?",db.Table))
		defer st.Close()
		if err != nil{
			return nil
		}
		res,err := st.Query(id)
		defer res.Close()
		if err != nil{
			return nil
		}

		if res.Next(){
			var id int64
			var key string
			var val string
			var name string
			var pid int64
			var addtime int64
			err := res.Scan(&id,&key,&val,&pid,&addtime,&name)
			if err != nil{
				return nil
			}
			return map[string]interface{}{"id":id,"key":key,"val":val,"name":name,"pid":pid}
		}
		return nil
	}
	/**
	  查询一条配置信息
	 */
	func (db *DbConf) FindConfById(id int64,pval string) string  {

		st,err := db.Db.Prepare(fmt.Sprintf("SELECT * from %s WHERE id=?",db.Table))
		defer st.Close()
		if err != nil{
			return ""
		}
		res,err := st.Query(id)
		defer res.Close()
		if err != nil{
			return ""
		}
		if res.Next(){
			var id int64
			var key string
			var val string
			var pid int64
			var addtime int64
			err := res.Scan(&id,&key,&val,&pid,&addtime)
			if err != nil{
				return ""
			}
			if pid == 0{
				return fmt.Sprintf("%s/%s",key,pval)
			}else if pval != ""{
				return db.FindConfById(pid,fmt.Sprintf("%s/%s",key,pval))
			}else{
				return db.FindConfById(pid,fmt.Sprintf("%s:::%s",key,val))
			}
		}
		return ""
	}

	func (db *DbConf) FindAll(preid int64,prekey string) []string  {
		//db.Lock.Lock()
		//defer db.Lock.Unlock()
		st,err := db.Db.Prepare(fmt.Sprintf("SELECT * from %s WHERE pid=?",db.Table))
		defer st.Close()
		if err != nil{
			fmt.Printf("查询数据失败1%v",err)
			return nil
		}
		res,err := st.Query(preid)
		defer res.Close()
		if err != nil{
			fmt.Printf("查询数据失败2%v",err)
			return nil
		}
		len := 0
		retList := make([]string,0)
		for res.Next(){
			len++
			var id int64
			var key string
			var val string
			var name string
			var pid int64
			var addtime int64
			err := res.Scan(&id,&key,&val,&pid,&addtime,&name)
			fmt.Printf("ID:%d Key:%s Val:%s Pid: %d\n",id,key,val,pid)
			if err != nil{
				fmt.Printf("查询数据失败3%v",err)
				return nil
			}
			var npre string = fmt.Sprintf("%s/%s",prekey,key)
			if prekey == ""{
				npre = key
			}
			sub := db.FindAll(id,npre)
			if sub == nil{
				if prekey == ""{
					retList = append(retList,fmt.Sprintf("%s:::%s",key,val))
				}else{
					retList = append(retList,fmt.Sprintf("%s/%s:::%s",prekey,key,val))
				}

			}else{
				for i := range sub{
					fmt.Printf("PreKey: %s Pid:%d Index : %d -> %s \n",prekey,preid,i,sub[i])
					//if "" != sub[i]{
						retList = append(retList,sub[i])
					//}

				}
			}
		}
		if len == 0{
			return nil
		}
		return retList
	}

	func (db *DbConf) Close()  {
		db.Db.Close()
	}

	func initConfDb(dbdir string)  {
		db,err := sql.Open("sqlite3",fmt.Sprintf("%s/Base.db",dbdir))
		if err != nil{
			fmt.Println("打开数据库失败",err)
		}
		ConfCon = DbConf{db,"Conf",new(sync.Mutex)}
	}




/********************************* TCP操作 *********************************/
//发送全量的配置给代理
func SendAllConfToAgent(tcpCon *net.TCPConn){
	fmt.Println("发送全部配置")
	vlist:=ConfCon.FindAll(0,"")
	if vlist != nil{
		for i:=range vlist{
			fmt.Println(fmt.Sprintf("A:%s \r\n",vlist[i]))
			tcpCon.Write([]byte(fmt.Sprintf("A:%s\r\n",vlist[i])))
		}
	}
}
/**
 发送数据给全部的客户端
 */
func SendUpdateDataToAllClient(nid int64,action,name string)  {
	fmt.Printf("发送新数据 %d %s\n",nid,name)
	menu := make(map[int64]string)
	var data string
	node := ConfCon.FindConfByIdMap(nid)
	pid := nid
	for pid > 0{
		id,npid,key1 := ConfCon.FindById(pid)
		fmt.Printf("查询菜单: %d %d %s\n",id,pid,key1)
		if id > 0{
			menu[id] = key1
		}
		pid = npid
	}
	key := ""
	for _,k := range menu{
		if key == ""{
			key = k
		}else{
			key = k + "/" + key
		}

	}

	if name != ""{
		key += "/" + name
		data = fmt.Sprintf("D:%s\r\n",key)
	}else{

		if node == nil{
			return
		}

		data = fmt.Sprintf("%s:%s:::%s\r\n",action,key,node["val"])
	}



	fmt.Printf("发送数据给客户端:%s %d\n",data,len(AgentClientMap))
	for _,k := range AgentClientMap{

		k.Write([]byte(data))
	}
}
//这里就是在处理心跳的问题
func TcpClientHandl(tcpCon *net.TCPConn){
	f,err :=tcpCon.File()
	if err != nil{
		return
	}
	AgentClientMap[f.Fd()] = tcpCon

	defer delete(AgentClientMap, f.Fd())
 	defer tcpCon.Close()
 	SendAllConfToAgent(tcpCon)
	bufferIo := bufio.NewReader(tcpCon)
	for{
		_,err := bufferIo.ReadByte()
		if err != nil {
			//这里因为做了心跳，所以就没有加deadline时间，如果客户端断开连接
			//这里ReadByte方法返回一个io.EOF的错误，具体可考虑文档
			if err == io.EOF {
				fmt.Printf("client %s is close!\n",tcpCon.RemoteAddr().String())
			}
			//在这里直接退出goroutine，关闭由defer操作完成
			return
		}
		tcpCon.Write([]byte("H:    \r\n")) //心跳
	}
}
func TcpServer(port int){
	tcpAddr ,err := net.ResolveTCPAddr("tcp",fmt.Sprintf("0.0.0.0:%d",port))
	CheckError(err)
	tcpServer,error := net.ListenTCP("tcp",tcpAddr)
	CheckError(error)
	for{
		conTcp,error := tcpServer.AcceptTCP()
		CheckError(error)
		go TcpClientHandl(conTcp)//发送全部的配置信息给代理端

	}
}
/********************************* HTTP *********************************/
//发送静态资源
func httpStatic(w http.ResponseWriter, req *http.Request){
	url := strings.ToLower(req.URL.Path)
	if strings.HasSuffix(url,".js"){
		w.Header().Set("Content-Type","application/javascript")
	}else if strings.HasSuffix(url,".css") {
		w.Header().Set("Content-Type","text/css")
	}else if strings.HasSuffix(url,".png") {
		w.Header().Set("Content-Type","image/png")
	}else if strings.HasSuffix(url,".jpg") {
		w.Header().Set("Content-Type","image/jpeg")
	}else if strings.HasSuffix(url,".jpeg") {
		w.Header().Set("Content-Type","image/jpeg")
	}else if strings.HasSuffix(url,".gif") {
		w.Header().Set("Content-Type","image/gif")
	}else{
		w.Header().Set("Content-Type","application/octet-stream")
	}
	//w.Write(GetContent(url))
}
func WebIndex(w http.ResponseWriter, req *http.Request){
	sess:=SessionInit(w,req)
	uid:=sess.GetSession("uid")
	if uid == nil{
		if req.Method == "GET"{
			WebDisplayHtml(w,"index",nil)
		}else{
			form := InitPostForm(req)
			if "admin" == form.GPost("name","") && "123456" == form.GPost("pwd",""){
				sess.SetSession("uid",1)
				http.Redirect(w,req,"/main",http.StatusFound)
			}else{
				WebDisplayHtml(w,"index",nil)
			}
		}

	}else{
		http.Redirect(w,req,"/main",http.StatusFound)
	}

}
func WebMain(w http.ResponseWriter, req *http.Request)  {
	WebCheckoutLogin(w,req)
	pid ,_:= strconv.ParseInt(req.URL.Query().Get("id"),10,64)
	opid := pid
	data:=ConfCon.GetConfByPid(pid)

	menu:= make(map[int64]string)
	fmt.Printf("参数是啥%d\n",pid)
	for pid > 0{
		id,npid,name := ConfCon.FindById(pid)
		fmt.Printf("参数是啥%d : %d %d %s\n",pid,id,npid,name)
		if id > 0{
			menu[id] = name
		}
		pid = npid
	}
	d,_ := json.Marshal(menu)
	fmt.Printf("JSON: %s\n",d)
	WebDisplayHtml(w,"main",map[string]interface{}{"list":data,"menu":menu,"pid":opid})
}
func WebAdd(w http.ResponseWriter, req *http.Request)  {
	WebCheckoutLogin(w,req)
	GForm := InitPostForm(req)
	pid ,_:= strconv.ParseInt(GForm.GPost("pid","0"),10,64)
	key := GForm.GPost("key","")
	val := GForm.GPost("val","")
	name := GForm.GPost("name","")
	if ConfCon.AddConf(key,val,name,pid) == 0{
		JsonFail(w,1,"创建配置失败")
		return
	}
	JsonSuccess(w,nil)
}

func WebDelNode(w http.ResponseWriter, req *http.Request){
	WebCheckoutLogin(w,req)
	id ,_:= strconv.ParseInt(req.URL.Query().Get("id"),10,64)
	rows := ConfCon.DelConf(id)
	if rows > 0 {
		JsonSuccess(w,nil)
	}else{
		JsonFail(w,1,"删除失败")
	}

}
func WebGetNode(w http.ResponseWriter, req *http.Request){
	WebCheckoutLogin(w,req)
	id ,_:= strconv.ParseInt(req.URL.Query().Get("id"),10,64)
	data := ConfCon.FindConfByIdMap(id)
	if data != nil{
		JsonSuccess(w,data)
	}else{
		JsonFail(w,1,"配置不存在")
	}

}
func WebUpdateNode(w http.ResponseWriter, req *http.Request){
	WebCheckoutLogin(w,req)
	GForm := InitPostForm(req)
	id ,_:= strconv.ParseInt(GForm.GPost("id","0"),10,64)
	key := GForm.GPost("key","")
	name := GForm.GPost("name","")
	val := GForm.GPost("val","")
	if ConfCon.Update(id,key,name,val) == 0{
		JsonFail(w,1,"创建配置失败")
		return
	}
	JsonSuccess(w,nil)

}

func WebCheckoutLogin(w http.ResponseWriter, req *http.Request){
	sess:=SessionInit(w,req)
	uid:=sess.GetSession("uid")
	if uid == nil{
		http.Redirect(w,req,"/",http.StatusFound)
	}

}
//HttpServer
func HttpServer(port int)  {
	http.HandleFunc("/static/",httpStatic)
	http.HandleFunc("/",WebIndex)
	http.HandleFunc("/main",WebMain)
	http.HandleFunc("/add",WebAdd)
	http.HandleFunc("/delete",WebDelNode)
	http.HandleFunc("/get",WebGetNode)
	http.HandleFunc("/update",WebUpdateNode)
	http.ListenAndServe(fmt.Sprintf(":%d",port),nil)
}

func WebDisplayHtml(w http.ResponseWriter,templateName string,data interface{})  {
	w.Header().Set("Content-Type","text/html")
	tem,_:=template.ParseFiles("/Users/wenba/go/src/GConf/template/index.html")
	tem.ExecuteTemplate(w,templateName,data)
}

func WebDisplayJson(w http.ResponseWriter,data interface{})  {
	w.Header().Set("Content-Type","application/json")
	d,_ := json.Marshal(data)
	w.Write([]byte(d))
}

func JsonSuccess(w http.ResponseWriter,data interface{})  {
	dd := GHttpJson{0,"",data}
	WebDisplayJson(w,dd)
}
func JsonFail(w http.ResponseWriter,code int64,msg string)  {
	dd := GHttpJson{code,msg,nil}
	WebDisplayJson(w,dd)
}

func InitPostForm(req *http.Request) GHttpPostForm {
	req.ParseForm()
	return GHttpPostForm{req}
}
type GHttpPostForm struct {
	Req *http.Request
}
type GHttpJson struct {
	Code int64
	Msg string
	Data interface{}
}
func (this GHttpPostForm) GPost(key,defval string) string {
	if ret,ok := this.Req.Form[key];ok{
		return ret[0]
	}
	return defval
}
/********************************* Session *********************************/
	type GSession struct {
		Rep http.ResponseWriter
		Req *http.Request
		Sid string
	}
	type GSessionNode struct {
		Expire int64
		Data map[string]interface{}
	}
	var GSessionData map[string]GSessionNode = make(map[string]GSessionNode)

	func Int64ToBytes(i int64) []byte {
		var buf = make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(i))
		return buf
	}
	func SessionInit(w http.ResponseWriter, req *http.Request) GSession {
		ret := GSession{w,req,""}
		cook ,err := req.Cookie("sid")
		if err == http.ErrNoCookie{
			h:= md5.New()
			//fmt.Printf("New CookieSid:%s\n",Int64ToBytes(time.Now().Unix().(string))
			h.Write([]byte(Int64ToBytes(time.Now().Unix())))
			ret.Sid = hex.EncodeToString(h.Sum(nil))
			GSessionData[ret.Sid] = GSessionNode{time.Now().Unix() + 600, make(map[string]interface{})}
			cooki := http.Cookie{Name:"sid",Value:ret.Sid,Path:"/",MaxAge:600}
			http.SetCookie(w,&cooki)
			return ret
		}
		fmt.Printf("CookieSid:%s\n",cook.Value)
		ret.Sid = cook.Value
		//GSessionData[ret.Sid] = GSessionNode{time.Now().Unix() + 600, make(map[string]interface{})}
		cooki := http.Cookie{Name:"sid",Value:ret.Sid,Path:"/",MaxAge:600}
		http.SetCookie(w,&cooki)
		return ret
	}
	func SessionGc()  {
		fmt.Println("Session:GC:回收过期的SESSION")
		t := time.Now().Unix()
		for k,v := range GSessionData{
			if v.Expire < t{
				delete(GSessionData,k)
			}
		}

	}

	func SessionGcHand()  {
		t := time.Tick(300 * time.Second)
		for {
			select{
			case <-t:
				SessionGc()
			}
		}
	}

	func (this GSession) SetSession(key string,val interface{})  {
		d,_ := json.Marshal(GSessionData)
		fmt.Printf("Session:%s\n",d)
		if node,ok := GSessionData[this.Sid];ok{
			fmt.Printf("data:%s %v Key:%s\n",this.Sid,node,key)
			node.Data[key] = val
			node.Expire = time.Now().Unix() + 600
		}else{
			aa:= make(map[string]interface{})
			aa[key]=val
			GSessionData[this.Sid] = GSessionNode{time.Now().Unix() + 600, aa}
		}
		d1,_ := json.Marshal(GSessionData)
		fmt.Printf("Session:%s\n",d1)
	}
	func (this GSession) GetSession(key string) interface{} {
		fmt.Printf("Sid:%s\n",this.Sid)
		if node,ok := GSessionData[this.Sid];ok{
			if val,tok := node.Data[key];tok{
				return val
			}
		}
		return nil
	}
	func (this GSession) DeleteSession(key string,val interface{})  {
		if node,ok := GSessionData[this.Sid];ok{
			if _,tok := node.Data[key];tok{
				delete(node.Data,key)
			}
		}
	}
	func (this GSession) DestorySession()  {
		if _,ok := GSessionData[this.Sid];ok{
			delete(GSessionData,this.Sid)
		}
	}
////////////////////////
func main() {
	//AgentList := make([]*net.TCPConn)
	//config
	cfg, err := config.ReadDefault("/Users/wenba/go/src/GConf/server.ini")
	if err != nil {
		fmt.Println("Fail to find server.ini %v",  err)
		os.Exit(-1)
	}
	initConfDb(GetConfString(cfg,"db.dir","/tmp"))
	RpcPort := GetConfInt(cfg,"rpc.port",8081)
	go TcpServer(RpcPort)
	HttpPort := GetConfInt(cfg,"http.port",8080)
	go SessionGcHand()
	HttpServer(HttpPort)

	//TemplateDir := GetConfString(cfg,"template.dir","/tmp")
	//DbDir = GetConfString(cfg,"db.dir","/tmp")
	//fmt.Println(HttpPort)
}
