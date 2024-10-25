package sql

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"sparrowhawktech/toolkit/util"
)

var timeType = reflect.TypeOf(time.Time{})
var byteArrayType = reflect.TypeOf([]byte{})
var rawMessageType = reflect.TypeOf(json.RawMessage{})

type DatasourceConfig struct {
	DriverName  *string `json:"driverName"`
	Name        *string `json:"name"`
	MaxIdle     *int    `json:"maxIdle"`
	MaxOpen     *int    `json:"maxOpen"`
	MaxLifetime *int    `json:"maxLifetime"`
}

func (o *DatasourceConfig) Validate() {
	if o.DriverName == nil {
		panic("Invalid databaseDriver")
	}
	if o.Name == nil {
		panic("Invalid datasourceName")
	}
}

type Databases struct {
	dbMap map[string]*sql.DB
	mux   *sync.Mutex
}

func (o *Databases) OpenDB(datasourceConfig DatasourceConfig) *sql.DB {
	key := fmt.Sprintf("%s.%s", *datasourceConfig.DriverName, *datasourceConfig.Name)
	o.mux.Lock()
	defer o.mux.Unlock()
	db, ok := o.dbMap[key]
	if !ok {
		db = OpenDB(datasourceConfig)
		o.dbMap[key] = db
	}
	return db
}

func (o *Databases) CloseDB(datasourceConfig DatasourceConfig) {
	key := fmt.Sprintf("%s.%s", *datasourceConfig.DriverName, *datasourceConfig.Name)
	o.mux.Lock()
	defer o.mux.Unlock()
	db, ok := o.dbMap[key]
	if ok {
		delete(o.dbMap, key)
		go CloseDB(db)
	}
}

var GlobalDatabases = &Databases{dbMap: make(map[string]*sql.DB), mux: &sync.Mutex{}}

func CloseDB(db *sql.DB) {
	util.ProcessError(db.Close())
}

func RollbackOnPanic(tx *sql.Tx) {
	if r := recover(); r != nil {
		err := tx.Rollback()
		if err != nil {
			util.ProcessError(err)
		}
		panic(r)
	}
}

func OpenDB(config DatasourceConfig) *sql.DB {
	db, err := sql.Open(*config.DriverName, *config.Name)
	util.CheckErr(err)
	if config.MaxIdle != nil {
		db.SetMaxIdleConns(*config.MaxIdle)
	}
	if config.MaxOpen != nil {
		db.SetMaxOpenConns(*config.MaxOpen)
	}
	if config.MaxLifetime != nil {
		db.SetConnMaxLifetime(time.Duration(*config.MaxLifetime) * time.Second)
	}
	return db
}

func PrepareStmt(tx *sql.Tx, sql string) *sql.Stmt {
	stmt, err := tx.Prepare(sql)
	util.CheckErr(err)
	return stmt
}

func QuerySingletonStmt(stmt *sql.Stmt, fields []interface{}, args ...interface{}) bool {
	r, err := stmt.Query(args...)
	util.CheckErr(err)
	defer closeRows(r)
	if r.Next() {
		util.CheckErr(r.Scan(fields...))
		return true
	} else {
		return false
	}
}

func ExecStmt(stmt *sql.Stmt, args ...interface{}) *sql.Result {
	r, err := stmt.Exec(args...)
	util.CheckErr(err)
	return &r
}

func QueryStmt(stmt *sql.Stmt, args ...interface{}) *sql.Rows {
	r, err := stmt.Query(args...)
	util.CheckErr(err)
	return r
}

func Scan(r *sql.Rows, vars ...interface{}) {
	util.CheckErr(r.Scan(vars...))
}

func FindStructStmt(stmt *sql.Stmt, template interface{}, queryParams ...interface{}) interface{} {
	result := QueryStructStmt(stmt, template, queryParams...)
	value := reflect.ValueOf(result)
	if value.Len() == 0 {
		objectType := reflect.TypeOf(template)
		return reflect.New(reflect.PtrTo(objectType)).Elem().Interface()
	} else {
		o := value.Index(0)
		return o.Addr().Interface()
	}
}

func QueryStructStmt(stmt *sql.Stmt, template interface{}, queryParams ...interface{}) interface{} {
	objectType := reflect.TypeOf(template)
	fields := listStructFields(objectType, 0)
	r, e := stmt.Query(queryParams...)
	util.CheckErr(e)
	count := len(fields)
	cols, e := r.Columns()
	util.CheckErr(e)
	if len(cols) > count {
		panic("Result set column count greater than struct field count")
	}
	buffer := make([]interface{}, len(fields))
	for i := range fields {
		fieldType := fields[i].Type
		if fieldType.Elem() == rawMessageType {
			buffer[i] = reflect.New(reflect.PtrTo(byteArrayType)).Interface()
		} else {
			buffer[i] = reflect.New(fields[i].Type).Interface()
		}
	}
	arr := reflect.MakeSlice(reflect.SliceOf(objectType), 0, 0)
	for r.Next() {
		util.CheckErr(r.Scan(buffer...))
		object := reflect.New(objectType).Elem()
		bufferToFields(object, buffer, 0)
		arr = reflect.Append(arr, object)
	}
	util.CheckErr(r.Close())
	return arr.Interface()
}

func bufferToFields(object reflect.Value, buffer []interface{}, offset int) int {
	instanceType := object.Type()
	instance := object
	isPtrStruct := object.Kind() == reflect.Ptr
	if isPtrStruct {
		instanceType = object.Type().Elem()
		instance = reflect.Indirect(reflect.New(instanceType))
	}
	n := 0
	created := !isPtrStruct
	for i := 0; i < instanceType.NumField(); i++ {
		of := instance.Field(i)
		if !isArrayField(of.Type()) {
			n = bufferToField(object, buffer, offset, n, created, instance, of)
		}
	}
	return n
}

func bufferToField(object reflect.Value, buffer []interface{}, offset int, n int, created bool, instance reflect.Value, of reflect.Value) int {
	v := buffer[n+offset]
	indirect := reflect.ValueOf(v).Elem()
	if indirect.Kind() == reflect.Ptr && !indirect.IsNil() && !created {
		object.Set(instance.Addr())
		created = true
	}
	if isStructPtrField(of) {
		n += bufferToFields(of, buffer, n+offset)
	} else if isStructField(of) {
		n += bufferToFields(of, buffer, n+offset)
	} else if of.Type().Elem() == rawMessageType {
		v := indirect.Interface()
		if v == nil {
			of.Addr().SetBytes(nil)
		} else {
			b := v.(*[]byte)
			j := (*json.RawMessage)(b)
			of.Set(reflect.ValueOf(j))
		}
		n++
	} else {
		of.Set(indirect)
		n++
	}
	return n
}

func isArrayField(t reflect.Type) bool {
	kind := t.Kind()
	return (kind == reflect.Slice || kind == reflect.Array) && t != byteArrayType && t != rawMessageType
}

func isStructField(value reflect.Value) bool {
	valueType := value.Type().Elem()
	return value.Kind() == reflect.Struct &&
		valueType != timeType
}

func isStructPtrField(value reflect.Value) bool {
	if value.Kind() == reflect.Ptr {
		instanceType := value.Type().Elem()
		return instanceType.Kind() == reflect.Struct && instanceType != timeType
	} else {
		return false
	}
}

func listStructFields(structType reflect.Type, offset int) []reflect.StructField {
	fields := make([]reflect.StructField, 0)
	return addStructFields(structType, fields, offset)
}

func addStructFields(structType reflect.Type, fields []reflect.StructField, offset int) []reflect.StructField {
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}
	for i := offset; i < structType.NumField(); i++ {
		f := structType.Field(i)
		structPtr := false
		kind := f.Type.Kind()
		if kind == reflect.Ptr {
			instanceType := f.Type.Elem()
			if instanceType.Kind() == reflect.Struct &&
				instanceType != timeType {
				structPtr = true
				fields = addStructFields(instanceType, fields, 0)
			}
		}
		if !structPtr {
			if kind == reflect.Struct && f.Type != timeType {
				fields = addStructFields(f.Type, fields, 0)
			} else if !isArrayField(f.Type) {
				fields = append(fields, f)
			}
		}
	}
	return fields
}

type FieldInfo struct {
	HolderType  *reflect.Type
	StructField *reflect.StructField
}

func ExecStructStmt(stmt *sql.Stmt, data interface{}) int64 {
	return ExecStructStmtOff(stmt, data, 0)
}

func ExecStructStmtOff(stmt *sql.Stmt, data interface{}, offset int) int64 {
	objectType := reflect.TypeOf(data)
	fields := listStructFields(objectType, offset)
	buffer := make([]interface{}, len(fields))
	value := reflect.ValueOf(data)
	fieldsToBuffer(value, buffer, offset)
	r, err := stmt.Exec(buffer...)
	util.CheckErr(err)
	lastId, _ := r.LastInsertId()
	return lastId
}

func fieldsToBuffer(value reflect.Value, buffer []interface{}, offset int) {
	fields := buildObjectFields(value)
	for i := 0; i < len(buffer); i++ {
		f := fields[i+offset]
		buffer[i] = f.Interface()
	}
}

func buildObjectFields(value reflect.Value) []reflect.Value {
	fields := make([]reflect.Value, 0)
	for i := 0; i < value.NumField(); i++ {
		f := value.Field(i)
		if f.Kind() == reflect.Struct {
			fields = append(fields, buildObjectFields(f)...)
		} else {
			fields = append(fields, f)
		}
	}
	return fields
}

func ForInsert(template interface{}, offset int) string {
	objectType := reflect.TypeOf(template)
	buffer := bytes.NewBufferString("(")
	buffer.WriteString(forSelect(objectType, nil, offset))
	buffer.WriteString(") values(")
	n := 0
	for i := offset; i < objectType.NumField(); i++ {
		if n > 0 {
			buffer.WriteString(", ")
		}
		n++
		buffer.WriteString(fmt.Sprintf("$%d", n))
	}
	buffer.WriteString(")")
	return buffer.String()
}

func ForUpdate(template interface{}, offset int, firstNum int) string {
	objectType := reflect.TypeOf(template)
	buffer := bytes.NewBufferString("")
	for i := 0; i < objectType.NumField()-offset; i++ {
		if i > 0 {
			buffer.WriteString(", ")
		}
		field := objectType.Field(i + offset)
		if v, ok := field.Tag.Lookup("sql"); ok {
			buffer.WriteString(v)
		} else {
			buffer.WriteString(field.Name)
		}
		buffer.WriteString(fmt.Sprintf(" = $%d", i+firstNum))
	}
	return buffer.String()
}

func forSelect(objectType reflect.Type, alias *string, offset int) string {
	buffer := bytes.NewBufferString("")
	for i := 0; i < objectType.NumField()-offset; i++ {
		if i > 0 {
			buffer.WriteString(", ")
		}
		if alias != nil {
			buffer.WriteString(*alias)
			buffer.WriteString(".")
		}
		field := objectType.Field(i + offset)
		if v, ok := field.Tag.Lookup("sql"); ok {
			buffer.WriteString(v)
		} else {
			buffer.WriteString(field.Name)
		}
	}
	return buffer.String()
}

func ScanAll(rows *sql.Rows) []interface{} {
	result := make([]interface{}, 0)
	columns, err := rows.Columns()
	util.CheckErr(err)
	n := len(columns)
	r := 0
	references := make([]interface{}, n)
	pointers := make([]interface{}, n)
	for i := range references {
		pointers[i] = &references[i]
	}
	for rows.Next() {
		util.CheckErr(rows.Scan(pointers...))
		values := make([]interface{}, n)
		for i := range pointers {
			values[i] = *pointers[i].(*interface{})
		}
		result = append(result, values)
		r = r + 1
	}
	return result
}

func closeRows(r *sql.Rows) {
	err := r.Close()
	util.CheckErr(err)
}
