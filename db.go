package db

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/astaxie/beego"
	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	_ "github.com/mattn/go-sqlite3"

	//odbc链接库
	"github.com/axgle/mahonia"
	//_ "github.com/lunny/godbc"
)

type PageData struct {
	Total  int                 `json:"total"`
	Pages  int                 `json:"pages"`
	Rows   []map[string]string `json:"rows"`
	Footer []map[string]string `json:"footer"`
	Extra  string              `json:"extra"`
}

//X 数据引擎
var X *xorm.Engine
var dbPool = make(map[string]*xorm.Engine)
var xLock sync.Mutex //数据库修改并发锁

//初始化X数据库
func InitX() {
	var err error
	X, err = xorm.NewEngine("sqlite3", beego.AppPath+"/conf/app.dat")
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println("[X数据库连接成功]")
	}
}

/*
  从连接池删除某个连接
*/
func Del(conn string) {
	// 查找键值是否存在
	if v, ok := dbPool[conn]; ok {
		v.Close()
		delete(dbPool, conn)
	}
}

//生成新的数据库链接
func NewDb(constr string) *xorm.Engine {
	if constr == "" { //如果为空默认返回系统库
		return X
	}
	// 查找键值是否存在
	if v, ok := dbPool[constr]; ok {
		fmt.Println("-----数据库连接:[", constr, "]已存在...")
		return v
	} else {
		var con = First("select * from adm_conn where conn=?", constr)
		if con == nil || len(con) < 1 {
			return nil
		}
		fmt.Println(con)
		fmt.Println("-----数据库连接:[", constr, "]创建成功...")
		var XX *xorm.Engine
		var err error
		if con["dbtype"] == "sqlite" {
			XX, err = xorm.NewEngine("sqlite3", beego.AppPath+"/db/"+con["dbname"])
		} else if con["dbtype"] == "mssql" {
			XX, err = xorm.NewEngine("odbc", "driver={SQL Server};server="+con["server"]+","+con["port"]+";database="+con["dbname"]+";uid="+con["uid"]+";pwd="+con["pwd"]+";")
		} else if con["dbtype"] == "mssql2k" {
			XX, err = xorm.NewEngine("odbc", "driver={SQL Server};server="+con["server"]+","+con["port"]+";database="+con["dbname"]+";uid="+con["uid"]+";pwd="+con["pwd"]+";")
		} else {
			XX, err = xorm.NewEngine(con["dbtype"], con["uid"]+":"+con["pwd"]+"@tcp("+con["server"]+":"+con["port"]+")/"+con["dbname"]+"?charset=utf8")
		}
		if err != nil {
			return nil
		}
		dbPool[constr] = XX
		return XX
	}
}

//生成新的sqlite数据库链接,conn连接标识,dbname数据库名称
func NewDbSqlite(conn string, dbname string) *xorm.Engine {
	// 查找键值是否存在
	if v, ok := dbPool[conn]; ok {
		return v
	} else {
		var XX *xorm.Engine
		var err error
		XX, err = xorm.NewEngine("sqlite3", beego.AppPath+"/db/"+dbname)
		if err != nil {
			return nil
		}
		dbPool[conn] = XX
		return XX
	}
}

//生成新的mysql数据库链接,conn连接标识
func NewDbMysql(conn string, server, port, uid, pwd, dbname string) *xorm.Engine {
	// 查找键值是否存在
	if v, ok := dbPool[conn]; ok {
		return v
	} else {
		var XX *xorm.Engine
		var err error
		XX, err = xorm.NewEngine("mysql", uid+":"+pwd+"@tcp("+server+":"+port+")/"+dbname+"?charset=utf8")
		if err != nil {
			return nil
		}
		dbPool[conn] = XX
		return XX
	}
}

//生成新的mssql数据库链接,conn连接标识
func NewDbMssql(conn string, server, port, uid, pwd, dbname string) *xorm.Engine {
	// 查找键值是否存在
	if v, ok := dbPool[conn]; ok {
		return v
	} else {
		var XX *xorm.Engine
		var err error
		XX, err = xorm.NewEngine("odbc", "driver={SQL Server};server="+server+","+port+";database="+dbname+";uid="+uid+";pwd="+pwd+";")
		if err != nil {
			return nil
		}
		dbPool[conn] = XX
		return XX
	}
}

//Query SQL语句查询
func Query(sqlorArgs ...interface{}) []map[string]string {
	rsts, err := X.Query(sqlorArgs...)
	if err != nil {
		fmt.Println("db query error:", err.Error())
		return nil
	}
	rst := ParseByte(X.DriverName(), rsts)
	return rst
}

//Query2 SQL语句查询 --指定链接
func Query2(XX *xorm.Engine, sqlorArgs ...interface{}) []map[string]string {
	//fmt.Println("drivername:",XX.DriverName())
	rsts, err := XX.Query(sqlorArgs...)
	if err != nil {
		fmt.Println("db query2 error:", err.Error())
		fmt.Println("sql:", sqlorArgs)
		if strings.Contains(err.Error(), "通讯链接失败") {
			fmt.Println("通讯链接失败,重建所有链接...")
			dbPool = make(map[string]*xorm.Engine)
		}

		//--------------------------------------------------------------------------------------
		//如果这个方法执行超时x秒，则会记录日志
		var paramSlice []string
		for _, param := range sqlorArgs {
			var p = ""
			switch param.(type) {
			case string:
				p = param.(string)
			case int:
				p = fmt.Sprintf("%d", param)
			case int32:
				p = fmt.Sprintf("%d", param)
			case int64:
				p = fmt.Sprintf("%d", param)
			default:
				p = fmt.Sprintf("%s", param)
			}
			paramSlice = append(paramSlice, p)
		}
		_logparams := strings.Join(paramSlice, ",")
		defer TimeoutWarning(XX.DriverName()+"[Query2]", _logparams, time.Now(), float64(0.5))
		//--------------------------------------------------------------------------------------

		//记录debug日志
		var atime = time.Now().Format("2006-01-02 15:04:05")
		var ip = ""
		var log = fmt.Sprintf("%s %s", sqlorArgs, err.Error())
		Exec("insert into adm_log(mch_id,user_id,username,logtype,opertype,title,content,ip,addtime)values(?,?,?,?,?,?,?,?,?)",
			"", "", "", "系统日志", "SQL", XX.DriverName()+" Query2 查询错误", log, ip, atime,
		)

		return nil
	}
	rst := ParseByte(XX.DriverName(), rsts)
	return rst
}

//First SQL语句查询第一条记录
func First(sqlorArgs ...interface{}) map[string]string {
	rsts, err := X.Query(sqlorArgs...)
	if err != nil {
		fmt.Println("db first error:", err.Error(), sqlorArgs)
		return nil
	}
	rst := ParseByte(X.DriverName(), rsts)
	if len(rst) > 0 {
		return rst[0]
	}
	return nil
}

//First SQL语句查询第一条记录 --指定数据库链接
func First2(XX *xorm.Engine, sqlorArgs ...interface{}) map[string]string {
	//--------------------------------------------------------------------------------------
	//如果这个方法执行超时x秒，则会记录日志
	var paramSlice []string
	for _, param := range sqlorArgs {
		var p = ""
		switch param.(type) {
		case string:
			p = param.(string)
		case int:
			p = fmt.Sprintf("%d", param)
		case int32:
			p = fmt.Sprintf("%d", param)
		case int64:
			p = fmt.Sprintf("%d", param)
		default:
			p = fmt.Sprintf("%s", param)
		}
		paramSlice = append(paramSlice, p)
	}
	_logparams := strings.Join(paramSlice, ",")
	defer TimeoutWarning(XX.DriverName()+"[First2]", _logparams, time.Now(), float64(0.5))
	//--------------------------------------------------------------------------------------

	rsts, err := XX.Query(sqlorArgs...)
	if err != nil {
		fmt.Println("db first2 error:", err.Error(), sqlorArgs[0])
		if strings.Contains(err.Error(), "通讯链接失败") {
			fmt.Println("通讯链接失败,重建所有链接...")
			dbPool = make(map[string]*xorm.Engine)
		}
		//记录debug日志
		var atime = time.Now().Format("2006-01-02 15:04:05")
		var ip = ""
		var log = fmt.Sprintf("%s %s", sqlorArgs, err.Error())
		Exec("insert into adm_log(mch_id,user_id,username,logtype,opertype,title,content,ip,addtime)values(?,?,?,?,?,?,?,?,?)",
			"", "", "", "系统日志", "SQL", XX.DriverName()+" First2 查询错误", log, ip, atime,
		)

		return nil
	}
	rst := ParseByte(XX.DriverName(), rsts)
	if len(rst) > 0 {
		return rst[0]
	}
	return nil
}

//FirstOrNil 第一条或空
func FirstOrNil(sqlorArgs ...interface{}) map[string]string {
	rsts, err := X.Query(sqlorArgs...)
	if err != nil {
		//fmt.Println("db FirstOrNil error:", err.Error())
		//fmt.Println("sql:\r\n", sqlorArgs)
		return nil
	}
	rst := ParseByte(X.DriverName(), rsts)
	if len(rst) > 0 {
		return rst[0]
	}
	return nil
}

//Pager 分页查询,返回easyui分页数据结构
func Pager(page int, pageSize int, sqlorArgs ...interface{}) interface{} {
	//添加分页控制
	if page > 0 {
		page = page - 1
	}

	var args0 = sqlorArgs[0]
	var s = Substring(sqlorArgs[0].(string), "select", "from")

	//获取总记录数
	var s1 = strings.Replace(sqlorArgs[0].(string), s, " count(*) as counts ", -1)
	sqlorArgs[0] = s1
	var a1 = First(sqlorArgs...)
	var rows = 0
	if a1 != nil {
		rows, _ = strconv.Atoi(a1["counts"])
	}
	//fmt.Println("rows:",rows)

	//计算总页数
	var pages = rows / pageSize
	if rows%pageSize > 0 {
		pages++
	}
	//fmt.Println(" pages:",pages)
	//获取当前页数据
	sqlorArgs[0] = args0 //恢复原语句
	//增加分页条件
	sqlorArgs[0] = sqlorArgs[0].(string) + " limit " + strconv.Itoa(page*pageSize) + "," + strconv.Itoa(pageSize)
	var list = Query(sqlorArgs...)

	var rst struct {
		Total int                 `json:"total"`
		Pages int                 `json:"pages"`
		Rows  []map[string]string `json:"rows"`
	}
	rst.Total = rows
	rst.Pages = pages
	rst.Rows = list

	//fmt.Println("sqlorArgs:",sqlorArgs)

	return rst
}

//Pager2 分页查询,返回easyui分页数据结构
func Pager2(XX *xorm.Engine, page int, pageSize int, sqlorArgs ...interface{}) PageData {
	//--------------------------------------------------------------------------------------
	//如果这个方法执行超时x秒，则会记录日志
	var paramSlice []string
	for _, param := range sqlorArgs {
		var p = ""
		switch param.(type) {
		case string:
			p = param.(string)
		case int:
			p = fmt.Sprintf("%d", param)
		case int32:
			p = fmt.Sprintf("%d", param)
		case int64:
			p = fmt.Sprintf("%d", param)
		default:
			p = fmt.Sprintf("%s", param)
		}
		paramSlice = append(paramSlice, p)
	}
	_logparams := strings.Join(paramSlice, ",")
	defer TimeoutWarning(XX.DriverName()+"[Pager2]", _logparams, time.Now(), float64(0.5))
	//--------------------------------------------------------------------------------------

	//添加分页控制
	if page > 0 {
		page = page - 1
	}

	var args0 = sqlorArgs[0]
	//var s = Substring(sqlorArgs[0].(string), "select", "from")

	//获取总记录数
	//var s1 = strings.Replace(sqlorArgs[0].(string), s, " count(*) as counts ", -1)
	var s1 = `select count(*) as counts from (` + sqlorArgs[0].(string) + `) as a`
	//var s1 = strings.Replace(sqlorArgs[0].(string), s, " count(*) as counts ", -1)
	sqlorArgs[0] = s1
	var a1 = First2(XX, sqlorArgs...)
	var rows = 0
	if a1 != nil {
		rows, _ = strconv.Atoi(a1["counts"])
	}
	//fmt.Println("rows:",rows)

	//计算总页数
	var pages = rows / pageSize
	if rows%pageSize > 0 {
		pages++
	}
	//fmt.Println(" pages:",pages)
	//获取当前页数据
	sqlorArgs[0] = args0 //恢复原语句
	//增加分页条件
	sqlorArgs[0] = sqlorArgs[0].(string) + " limit " + strconv.Itoa(page*pageSize) + "," + strconv.Itoa(pageSize)
	var list = Query2(XX, sqlorArgs...)

	// var rst struct {
	// 	Total int                 `json:"total"`
	// 	Pages int                 `json:"pages"`
	// 	Rows  []map[string]string `json:"rows"`
	// }
	var rst PageData

	rst.Total = rows
	rst.Pages = pages
	rst.Rows = list

	//fmt.Println("sqlorArgs:",sqlorArgs)

	return rst
}

//Pager2 mssql分页查询,返回easyui分页数据结构
func Pager2MsSql(XX *xorm.Engine, page int, pageSize int, tbname string, sql string, where string, orderstr string, sqlorArgs ...interface{}) PageData {
	//添加分页控制
	if page == 0 {
		page = 1
	}

	var args0 = sqlorArgs[0]
	//获取总记录数
	var s1 = "select count(*) as counts from " + tbname + " " + where
	sqlorArgs[0] = s1
	var a1 = First2(XX, sqlorArgs...)
	var rows = 0
	if a1 != nil {
		rows, _ = strconv.Atoi(a1["counts"])
	}

	//计算总页数
	var pages = rows / pageSize
	if rows%pageSize > 0 {
		pages++
	}
	//fmt.Println(" pages:",pages)
	//获取当前页数据
	sqlorArgs[0] = args0 //恢复原语句
	//增加分页条件
	//sqlorArgs[0] = sqlorArgs[0].(string) + " limit " + strconv.Itoa(page*pageSize) + "," + strconv.Itoa(pageSize)
	//MSSQL专用查询

	var pp = pageSize * (page - 1)
	var mssql = `
		SELECT TOP ` + strconv.Itoa(pageSize) + ` * FROM ` + tbname + `
		WHERE id > (
			SELECT isnull(MAX(id),0) FROM (
				SELECT TOP ` + strconv.Itoa(pp) + ` id FROM ` + tbname + `  ` + where + `
			) as t
		)
		` + orderstr
	//fmt.Println("mssql:", mssql)
	sqlorArgs[0] = mssql
	var list = Query2(XX, sqlorArgs...)

	var rst PageData

	rst.Total = rows
	rst.Pages = pages
	rst.Rows = list

	//fmt.Println("sqlorArgs:",sqlorArgs)

	return rst
}

//Page 分页查询,分别返回数据、总页数、总记录数
func Page(page int, pageSize int, sqlorArgs ...interface{}) ([]map[string]string, int, int) {
	//添加分页控制
	if page > 0 {
		page = page - 1
	}
	var args0 = sqlorArgs[0]
	var s = Substring(sqlorArgs[0].(string), "select", "from")

	//获取总记录数
	var s1 = strings.Replace(sqlorArgs[0].(string), s, " count(*) as counts ", -1)
	sqlorArgs[0] = s1
	var a1 = First(sqlorArgs...)
	var rows = 0
	if a1 != nil {
		rows, _ = strconv.Atoi(a1["counts"])
	}
	//fmt.Println("rows:",rows)

	//计算总页数
	var pages = rows / pageSize
	if rows%pageSize > 0 {
		pages++
	}
	//fmt.Println(" pages:",pages)
	//获取当前页数据
	sqlorArgs[0] = args0 //恢复原语句
	//增加分页条件
	sqlorArgs[0] = sqlorArgs[0].(string) + " limit " + strconv.Itoa(page*pageSize) + "," + strconv.Itoa(pageSize)
	var list = Query(sqlorArgs...)

	return list, pages, rows
}

/*
Exec 执行SQL语句
*/
func Exec(sql string, Args ...interface{}) int64 {
	var index = 0
	rear := append([]interface{}{}, Args[index:]...)
	Args = append(Args[0:index], sql)
	Args = append(Args, rear...)

	//启用事务
	session := X.NewSession()
	defer session.Close()
	session.Begin()

	//为并发加锁
	//xLock.Lock()
	_, err := session.Exec(Args...)
	//xLock.Unlock()
	//_, err := X.Exec(Args...)

	if err != nil {
		beego.Error("sql错误:" + err.Error() + "\r\n" + sql)
		fmt.Println("db exec error:", sql, Args, err.Error())

		session.Rollback()
		return 0
	}

	err = session.Commit()
	if err != nil {
		return 0
	}
	return 1
}

/*
Exec 执行SQL语句 --指定数据库链接
*/
func Exec2(XX *xorm.Engine, sql string, Args ...interface{}) int64 {
	var index = 0
	rear := append([]interface{}{}, Args[index:]...)
	Args = append(Args[0:index], sql)
	Args = append(Args, rear...)

	//--------------------------------------------------------------------------------------
	//如果这个方法执行超时x秒，则会记录日志
	var paramSlice []string
	for _, param := range Args {
		var p = ""
		switch param.(type) {
		case string:
			p = param.(string)
		case int:
			p = fmt.Sprintf("%d", param)
		case int32:
			p = fmt.Sprintf("%d", param)
		case int64:
			p = fmt.Sprintf("%d", param)
		default:
			p = fmt.Sprintf("%s", param)
		}
		paramSlice = append(paramSlice, p)
	}
	_logparams := strings.Join(paramSlice, ",")
	defer TimeoutWarning(XX.DriverName()+"[Exec2]", sql+_logparams, time.Now(), float64(0.5))

	//启用事务
	session := XX.NewSession()
	defer session.Close()
	session.Begin()

	//为并发加锁
	xLock.Lock()
	_, err := session.Exec(Args...)
	xLock.Unlock()
	//_, err := XX.Exec(Args...)

	if err != nil {
		beego.Error("sql错误:" + err.Error() + "\r\n" + sql)
		fmt.Println("db exec2 error:", sql, Args, err.Error())
		if strings.Contains(err.Error(), "通讯链接失败") {
			fmt.Println("通讯链接失败,重建所有链接...")
			dbPool = make(map[string]*xorm.Engine)
		}
		session.Rollback()
		return 0
	}

	err = session.Commit()
	if err != nil {
		return 0
	}
	return 1
}

/*
Insert 执行插入语句
*/
func Insert(sql string, tb string, Args ...interface{}) int64 {
	var index = 0
	rear := append([]interface{}{}, Args[index:]...)
	Args = append(Args[0:index], sql)
	Args = append(Args, rear...)

	//启用事务
	session := X.NewSession()
	defer session.Close()
	session.Begin()

	//为并发加锁
	xLock.Lock()
	_, err := session.Exec(Args...)
	xLock.Unlock()
	//_, err := XX.Exec(Args...)

	if err != nil {
		beego.Error("sql错误:" + err.Error() + "\r\n" + sql)
		fmt.Println("db exec2 error:", sql, Args, err.Error())
		if strings.Contains(err.Error(), "通讯链接失败") {
			fmt.Println("通讯链接失败,重建所有链接...")
			dbPool = make(map[string]*xorm.Engine)
		}
		session.Rollback()
		return 0
	}

	sql = `select last_insert_rowid() as id from ` + tb
	if X.DriverName() == "sqlite" {
		sql = `SELECT last_insert_rowid() AS id`
	}
	if X.DriverName() == "mysql" {
		sql = `SELECT LAST_INSERT_ID() AS id`
	}
	if X.DriverName() == "mssql" {
		sql = `SELECT @@IDENTITY as id`
	}
	var r, err1 = session.Query(sql)

	if err1 != nil {
		session.Rollback()
		return 0
	}
	rst := ParseByte(X.DriverName(), r)
	if len(rst) < 1 {
		session.Rollback()
		return 0
	}
	var id = rst[0]["id"]
	var i, _ = strconv.ParseInt(id, 10, 64)

	//提交事务
	err = session.Commit()
	if err != nil {
		return 0
	}

	return i
}

/*
Insert 执行插入语句 --指定数据库链接
*/
func Insert2(XX *xorm.Engine, sql string, tb string, Args ...interface{}) int64 {
	var index = 0
	rear := append([]interface{}{}, Args[index:]...)
	Args = append(Args[0:index], sql)
	Args = append(Args, rear...)

	//--------------------------------------------------------------------------------------
	//如果这个方法执行超时x秒，则会记录日志
	var paramSlice []string
	for _, param := range Args {
		var p = ""
		switch param.(type) {
		case string:
			p = param.(string)
		case int:
			p = fmt.Sprintf("%d", param)
		case int32:
			p = fmt.Sprintf("%d", param)
		case int64:
			p = fmt.Sprintf("%d", param)
		default:
			p = fmt.Sprintf("%s", param)
		}
		paramSlice = append(paramSlice, p)
	}
	_logparams := strings.Join(paramSlice, ",")
	defer TimeoutWarning(XX.DriverName()+"[Insert2]", sql+_logparams, time.Now(), float64(0.5))
	//--------------------------------------------------------------------------------------

	//启用事务
	session := XX.NewSession()
	defer session.Close()
	session.Begin()

	//为并发加锁
	xLock.Lock()
	_, err := session.Exec(Args...)
	xLock.Unlock()
	//_, err := XX.Exec(Args...)

	if err != nil {
		beego.Error("sql错误:" + err.Error() + "\r\n" + sql)
		fmt.Println("db exec2 error:", sql, Args, err.Error())
		if strings.Contains(err.Error(), "通讯链接失败") {
			fmt.Println("通讯链接失败,重建所有链接...")
			dbPool = make(map[string]*xorm.Engine)
		}
		session.Rollback()

		//记录debug日志
		var atime = time.Now().Format("2006-01-02 15:04:05")
		var ip = ""
		var log = fmt.Sprintf("%s %s %s", sql, Args, err.Error())
		Exec("insert into adm_log(mch_id,user_id,username,logtype,opertype,title,content,ip,addtime)values(?,?,?,?,?,?,?,?,?)",
			"", "", "", "系统日志", "SQL", XX.DriverName()+" Insert2 查询错误", log, ip, atime,
		)

		return 0
	}

	sql = `select last_insert_rowid() as id from ` + tb
	if XX.DriverName() == "sqlite" {
		sql = `SELECT last_insert_rowid() AS id`
	}
	if XX.DriverName() == "mysql" {
		sql = `SELECT LAST_INSERT_ID() AS id`
	}
	if XX.DriverName() == "mssql" {
		sql = `SELECT @@IDENTITY as id`
	}
	var r, err1 = session.Query(sql)

	if err1 != nil {
		session.Rollback()
		return 0
	}
	rst := ParseByte(X.DriverName(), r)
	if len(rst) < 1 {
		session.Rollback()
		return 0
	}
	var id = rst[0]["id"]
	var i, _ = strconv.ParseInt(id, 10, 64)

	//提交事务
	err = session.Commit()
	if err != nil {
		return 0
	}

	return i
}

/*
ParseByte 字典字节类型转换
*/
func ParseByte(drivername string, beans []map[string][]byte) []map[string]string {
	_st := make([]map[string]string, 0)

	var dec mahonia.Decoder
	dec = mahonia.NewDecoder("gbk")

	for _, value := range beans {
		_mt := make(map[string]string)
		for k, v := range value {
			if drivername != "odbc" {
				_mt[strings.ToLower(k)] = strings.TrimSpace(string(v))
			} else {
				_mt[strings.ToLower(k)] = strings.TrimSpace(dec.ConvertString(string(v))) //解决mssql中文乱码问题
			}
		}
		_st = append(_st, _mt)
	}
	return _st
}

/*
Substring 提取子字符串
*/
func Substring(str, starting, ending string) string {
	s := strings.Index(str, starting)
	if s < 0 {
		return ""
	}
	s += len(starting)
	e := strings.Index(str[s:], ending)
	if e < 0 {
		return ""
	}
	return str[s : s+e]
}

/* MD5加密方式
data := []byte("admin")
has := md5.Sum(data)
md5str1 := fmt.Sprintf("%x", has) //将[]byte转成16进制
fmt.Println("md5:",md5str1)
*/
//随机字符串
func RandomString(lenght int) string {
	//str := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	//第一个字符非数字
	result := []byte{}
	str0 := "abcdefghijklmnopqrstuvwxyz"
	bytes0 := []byte(str0)
	r0 := rand.New(rand.NewSource(time.Now().UnixNano()))
	result = append(result, bytes0[r0.Intn(len(bytes0))])

	//剩下的随机数字+字母
	str := "0123456789abcdefghijklmnopqrstuvwxyz"
	bytes := []byte(str)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < lenght-1; i++ {
		result = append(result, bytes[r.Intn(len(bytes))])
	}
	return string(result)
}

//超时告警
func TimeoutWarning(tag, msg string, start time.Time, timeLimit float64) {
	dis := time.Now().Sub(start).Seconds()
	if dis > timeLimit {
		fmt.Println("运行超时告警：", tag, dis, "秒", tag, msg)
		//记录debug日志
		var atime = time.Now().Format("2006-01-02 15:04:05")
		var ip = ""
		var log = msg
		Exec("insert into adm_log(mch_id,user_id,username,logtype,opertype,title,content,ip,addtime)values(?,?,?,?,?,?,?,?,?)",
			"", "", "", "系统日志", "告警", tag+"[运行超时告警 "+fmt.Sprintf("%f", dis)+"秒]", log, ip, atime,
		)
	}
}
