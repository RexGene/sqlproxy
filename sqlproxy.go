package sqlproxy

import (
	"database/sql"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"time"
)

const (
	saveCmdMaxLen = 128
)

type FieldData struct {
	Name  string
	Value string
}

type QueryCmd struct {
	TableName  string
	FieldNames []string
	Condition  *FieldData
}

type SaveCmd struct {
	TableName string
	Fields    []*FieldData
	Condition *FieldData
	IsNew     bool
}

type SqlProxy struct {
	user        string
	password    string
	ip          string
	port        string
	dbName      string
	db          *sql.DB
	saveCmdList chan *SaveCmd
	quitEvent   chan int
}

func (this *SqlProxy) messageLoop() {
	for {
		select {
		case cmd := <-this.saveCmdList:
			this.SaveData(cmd)
		case <-this.quitEvent:
			return
		case <-time.After(time.Second):
		}
	}
}

func (this *SqlProxy) SaveData(cmd *SaveCmd) error {
	if this.db == nil {
		return errors.New("connection already disconnect")
	}

	var sqlStr string

	if cmd.IsNew {
		sqlStr = "insert into " + cmd.TableName
		fieldNamesStr := "("
		valuesStr := "("

		for i, fieldData := range cmd.Fields {
			if i == 0 {
				fieldNamesStr = fieldNamesStr + ""
				valuesStr = valuesStr + ""
			} else {
				fieldNamesStr = fieldNamesStr + ","
				valuesStr = valuesStr + ","
			}

			fieldNamesStr = fieldNamesStr + fieldData.Name
			valuesStr = valuesStr + "'" + fieldData.Value + "'"

		}

		fieldNamesStr = fieldNamesStr + ")"
		valuesStr = valuesStr + ")"

		sqlStr = sqlStr + fieldNamesStr + " values " + valuesStr
	} else {
		sqlStr = "update " + cmd.TableName + " set"
		for i, fieldData := range cmd.Fields {
			if i == 0 {
				sqlStr = sqlStr + " "
			} else {
				sqlStr = sqlStr + ","
			}

			sqlStr = sqlStr + fieldData.Name + " = '" + fieldData.Value + "'"
		}

		condition := cmd.Condition
		if condition != nil && condition.Name != "" {
			sqlStr = sqlStr + " where " + condition.Name + " = '" + condition.Value + "'"
		}
	}

	log.Println(sqlStr)
	_, err := this.db.Exec(sqlStr)
	if err != nil {
		return err
	}

	return nil
}

func NewSqlProxy(user string, password string, ip string, port string, dbName string) *SqlProxy {
	sqlProxy := &SqlProxy{
		user:     user,
		password: password,
		ip:       ip,
		port:     port,
		dbName:   dbName,
	}

	go sqlProxy.messageLoop()

	return sqlProxy
}

func (this *SqlProxy) GetSaveCmdList() chan<- *SaveCmd {
	return this.saveCmdList
}

func (this *SqlProxy) PushSaveCmd(saveCmd *SaveCmd) {
	this.saveCmdList <- saveCmd
}

func (this *SqlProxy) Connect() error {
	if this.db != nil {
		return errors.New("connection already connect")
	}

	connStr := this.user + ":" + this.password + "@tcp(" + this.ip + ":" + this.port + ")/" + this.dbName + "?charset=utf8"

	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return err
	}

	this.db = db
	this.saveCmdList = make(chan *SaveCmd, saveCmdMaxLen)
	this.quitEvent = make(chan int, 1)
	return nil
}

func (this *SqlProxy) Close() error {
	if this.db == nil {
		return errors.New("connection already disconnect")
	}

	this.db.Close()
	this.db = nil

	this.quitEvent <- 1

	return nil
}

func (this *SqlProxy) LoadData(queryData *QueryCmd) ([]map[string]string, error) {
	if this.db == nil {
		return nil, errors.New("connection already disconnect")
	}

	queryString := "select"
	for i, fieldName := range queryData.FieldNames {
		var delmiter string
		if i != 0 {
			delmiter = ", "
		} else {
			delmiter = " "
		}
		queryString = queryString + delmiter + fieldName
	}

	queryString = queryString + " from " + queryData.TableName
	condition := queryData.Condition
	if condition != nil && condition.Name != "" {
		queryString = queryString + " where " + condition.Name + " = '" + condition.Value + "'"
	}

	log.Println("query string:", queryString)

	rows, err := this.db.Query(queryString)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	dataMapList := make([]map[string]string, 0, 32)
	for rows.Next() {
		fieldNames := queryData.FieldNames
		dataMap := make(map[string]string)
		fieldLen := len(fieldNames)
		results := make([]string, fieldLen)
		interfaces := make([]interface{}, fieldLen)

		for i := 0; i < fieldLen; i++ {
			interfaces[i] = &results[i]
		}

		rows.Scan(interfaces...)

		for i, fieldName := range fieldNames {
			dataMap[fieldName] = results[i]
		}

		dataMapList = append(dataMapList, dataMap)
	}

	return dataMapList, nil
}
