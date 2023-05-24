package internal

import (
	"database/sql"
	// "encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

type DBClientinterface interface {
	Connect(user, pass, host string, port int, dbname string) error
	TranExec(string, ...interface{}) error
	Exec(string, ...interface{}) error
	UpdateDBVariables([]DBVariable) error
	Query(query string, dest interface{}, fields []string) error
	GetTableData(query string) ([]map[string]interface{}, error)
	Close() error
}

type DBVariable struct {
	key   string
	value string
}

type defaultDBClient struct {
	db *sql.DB
}

func NewDBClient() DBClientinterface {
	return &defaultDBClient{}
}

func (client *defaultDBClient) Connect(user, pass, host string, port int, dbname string) (err error) {

	// connStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?interpolateParams=true&timeout=5s",
	// 	user, pass, host, port, dbname)

	connStr := fmt.Sprintf("%s:%s@tcp(%s)/%s?interpolateParams=true&timeout=5s",
		user, pass, host, dbname)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to mysql: %s:%d, reason: %v", host, port, err)
	}

	err = db.Ping()
	if err != nil {
		return fmt.Errorf("failed ping database to mysql: %s:%d, reason: %v", host, port, err)
	}
	client.db = db
	return nil
}

func NewDBVariable(options map[string]string) (dbVariables []DBVariable) {

	for key, value := range options {
		varibale := DBVariable{}
		varibale.key = key
		varibale.value = value
		dbVariables = append(dbVariables, varibale)
	}
	return dbVariables
}

func (client *defaultDBClient) TranExec(query string, args ...interface{}) error {
	tx, err := client.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %v", err)
	}

	_, err = tx.Exec(query, args...)
	if err != nil {
		err = fmt.Errorf("exec query error: %s, reason: %v", query, err)

		if errT := tx.Rollback(); errT != nil {
			return fmt.Errorf("%v, rollback error:%v,", err.Error(), errT)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction error: %s, reason: %v", query, err)
	}

	return nil
}

func (client *defaultDBClient) Exec(query string, args ...interface{}) error {

	_, err := client.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("exec query error: %s, reason: %v", query, err)
	}

	return nil
}

func (client *defaultDBClient) UpdateDBVariables(dbVariables []DBVariable) error {
	query := "SET GLOBAL ?=?;"
	return client.updateVariables(query, dbVariables)
}

func (client *defaultDBClient) updateVariables(query string, dbVariables []DBVariable) error {

	for _, dbVariable := range dbVariables {
		key := strings.ReplaceAll(dbVariable.key, "-", "_")
		value := dbVariable.value
		if strings.HasPrefix(key, "loose_") {
			key = strings.Replace(dbVariable.key, "loose_", "", 1)
		}
		newQuery := strings.Replace(query, "?", key, 1)
		args := []interface{}{}
		s, conErr := strconv.ParseInt(value, 10, 64)
		if conErr == nil {
			args = append(args, s)
		} else {
			args = append(args, value)
		}

		err := client.Exec(newQuery, args...)
		if err != nil {
			return err
		}
	}

	return nil
}

//Query  Execute the query statement and return the result to dest
// params
// query : Query statement as: select * from test
// dest: Copy the query value to dest, which requires a slice structure of pointer type
// fields: Query Selected Fields， as: select name,age from test, fields = []string{"name","age"},
// If field is blank, select all fields of the structure for assignment
func (client *defaultDBClient) Query(query string, dest interface{}, fields []string) error {

	T := reflect.ValueOf(dest)
	if T.Kind() != reflect.Ptr {
		return fmt.Errorf("dest must be a pointer type slice struct")
	}
	T = reflect.Indirect(T)
	if T.Kind() != reflect.Slice {
		return fmt.Errorf("dest must be a pointer type slice struct")
	}
	item := reflect.New(T.Type().Elem()).Elem()
	if item.Kind() != reflect.Struct {
		return fmt.Errorf("dest must be a pointer type slice struct")
	}
	// query
	result, err := client.db.Query(query)
	if err != nil {
		return err
	}

	FieldsParse := []interface{}{}
	if len(fields) == 0 || (len(fields) > 0 && fields[0] == "*") {
		for index := 0; index < item.NumField(); index++ {
			FieldsParse = append(FieldsParse, item.Field(index).Addr().Interface())
		}
	} else {
		for _, field := range fields {
			fieldExist := false
			for index := 0; index < item.NumField(); index++ {
				f := item.Type().Field(index)
				fieldTag := strings.Split(f.Tag.Get("json"), ",")
				if len(fieldTag) > 0 && field == fieldTag[0] {
					FieldsParse = append(FieldsParse, item.Field(index).Addr().Interface())
					fieldExist = true
					break
				}
			}
			// If the field does not exist, an error is returned
			if !fieldExist {
				return fmt.Errorf("the json tag of field %s in struct %v does not exist", field, item.Type())
			}
		}
	}

	T1 := reflect.Indirect(T)
	for result.Next() {
		err := result.Scan(FieldsParse...)
		if err != nil {
			return err
		}
		itemTemp := reflect.New(T.Type().Elem()).Elem()
		itemTemp.Set(item)
		T = reflect.Append(T, itemTemp)

	}

	T1.Set(T)
	return nil
}

func (client *defaultDBClient) GetTableData(query string) ([]map[string]interface{}, error) {

	rows, err := client.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	count := len(columns)
	tableData := make([]map[string]interface{}, 0)
	values := make([]interface{}, count)
	valuePtrs := make([]interface{}, count)
	for rows.Next() {
		for i := 0; i < count; i++ {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		entry := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			entry[col] = v
		}
		tableData = append(tableData, entry)
	}
	// jsonData, err := json.Marshal(tableData)
	// if err != nil {
	// 	return nil, err
	// }
	// fmt.Println(string(jsonData))
	return tableData, nil

}

func (client *defaultDBClient) Close() error {
	return client.db.Close()
}
