package tx

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"reflect"

	sql2 "sparrowhawktech/toolkit/sql"
	"sparrowhawktech/toolkit/util"
)

type Future func()

type Transaction struct {
	datasourceConfig sql2.DatasourceConfig
	tx               *sql.Tx
	db               *sql.DB
	stmtMap          map[string]*sql.Stmt
	insMap           map[string]*sql.Stmt
	autoIdMap        map[string]*sql.Stmt
	updMap           map[string]*sql.Stmt
	delMap           map[string]*sql.Stmt
	sequences        *sql2.Sequences
	future           []Future
}

func (o *Transaction) Tx() *sql.Tx {
	return o.tx
}

func (o *Transaction) Db() *sql.DB {
	return o.db
}

func (o *Transaction) Seq() *sql2.Sequences {
	return o.sequences
}

func (o *Transaction) FindMapped(template interface{}, sql string, queryParams ...interface{}) interface{} {
	stmt := o.resolveStmt(sql)
	return sql2.FindStructStmt(stmt, template, queryParams...)
}

func (o *Transaction) QueryMapped(template interface{}, sql string, queryParams ...interface{}) interface{} {
	stmt := o.resolveStmt(sql)
	return sql2.QueryStructStmt(stmt, template, queryParams...)
}

func (o *Transaction) QueryMappedStmt(stmt *sql.Stmt, template interface{}, queryParams ...interface{}) interface{} {
	return sql2.QueryStructStmt(stmt, template, queryParams...)
}

func (o *Transaction) InsertMapped(schema string, data interface{}) int64 {
	offset := 0
	name := reflect.TypeOf(data).Name()
	key := schema + "." + name
	stmt, ok := o.insMap[key]
	if !ok {
		buf := bytes.NewBufferString("insert into")
		util.WriteString(schema, buf)
		util.WriteString(".", buf)
		util.WriteString(name, buf)
		sql2.ForInsert(data, offset, buf)
		sentence := buf.String()
		var err error
		stmt, err = o.tx.Prepare(sentence)
		util.CheckErr(err)
		o.insMap[key] = stmt
	}
	return o.ExecMappedStmt(stmt, data, offset)
}

func (o *Transaction) UpdateMapped(schema string, entity interface{}) int64 {
	objectType := reflect.TypeOf(entity)
	name := objectType.Name()
	key := schema + "." + name
	stmt, ok := o.updMap[key]
	if !ok {
		buf := bytes.NewBufferString("update")
		util.WriteString(schema, buf)
		util.WriteString(".", buf)
		util.WriteString(name, buf)
		util.WriteString(" set ", buf)
		sql2.ForUpdate(entity, 1, 2, buf)
		util.WriteString(" where ", buf)
		util.WriteString(o.resolveIdName(objectType), buf)
		util.WriteString(" = $1", buf)
		var err error
		stmt, err = o.tx.Prepare(buf.String())
		util.CheckErr(err)
		o.updMap[key] = stmt
	}
	return o.ExecMappedStmt(stmt, entity, 0)
}

func (o *Transaction) resolveIdName(objectType reflect.Type) string {
	idField, ok := objectType.FieldByName("Id")
	if !ok {
		panic(fmt.Sprintf("Id field not found for %s", objectType.Name()))
	}
	idName, ok := idField.Tag.Lookup("sql")
	if ok {
		return idName
	} else {
		return "Id"
	}
}

func (o *Transaction) DeleteMapped(schema string, entity interface{}) {
	objectType := reflect.TypeOf(entity)
	name := objectType.Name()
	key := schema + "." + name
	stmt, ok := o.delMap[key]
	if !ok {
		idName := o.resolveIdName(objectType)
		buf := bytes.NewBufferString("delete from ")
		util.WriteString(schema, buf)
		util.WriteString(".", buf)
		util.WriteString(name, buf)
		util.WriteString(" where ", buf)
		util.WriteString(idName, buf)
		util.WriteString(" = $1", buf)
		var err error
		stmt, err = o.tx.Prepare(buf.String())
		util.CheckErr(err)
		o.delMap[key] = stmt
	}
	o.ExecStmt(stmt, reflect.ValueOf(entity).FieldByName("Id").Interface())
}

func (o *Transaction) ExecMapped(sql string, data interface{}, offset ...int) int64 {
	stmt := o.resolveStmt(sql)
	return o.ExecMappedStmt(stmt, data, offset...)
}

func (o *Transaction) ExecMappedStmt(stmt *sql.Stmt, data interface{}, varOffset ...int) int64 {
	if len(varOffset) > 0 {
		return sql2.ExecStructStmtOff(stmt, data, varOffset[0])
	} else {
		return sql2.ExecStructStmt(stmt, data)
	}
}

func (o *Transaction) Exec(sql string, args ...interface{}) *sql.Result {
	stmt := o.resolveStmt(sql)
	return sql2.ExecStmt(stmt, args...)
}

func (o *Transaction) ExecStmt(stmt *sql.Stmt, args ...interface{}) *sql.Result {
	return sql2.ExecStmt(stmt, args...)
}

func (o *Transaction) Query(sql string, args ...interface{}) *sql.Rows {
	stmt := o.resolveStmt(sql)
	return sql2.QueryStmt(stmt, args...)
}

func (o *Transaction) Singleton(sql string, fields []interface{}, args ...interface{}) bool {
	stmt := o.resolveStmt(sql)
	return sql2.QuerySingletonStmt(stmt, fields, args...)
}

func (o *Transaction) resolveStmt(sql string) *sql.Stmt {
	stmt, ok := o.stmtMap[sql]
	if !ok {
		var err error
		stmt, err = o.tx.Prepare(sql)
		util.CheckErr(err)
		o.stmtMap[sql] = stmt
	}
	return stmt
}

func (o *Transaction) AddFuture(f func()) {
	if o.future == nil {
		o.future = make([]Future, 1)
		o.future[0] = f
	} else {
		o.future = append(o.future, f)
	}
}

func NewTransaction(datasourceConfig sql2.DatasourceConfig, tx *sql.Tx, db *sql.DB) *Transaction {
	sequences := sql2.NewSequences(datasourceConfig, tx)
	trx := Transaction{tx: tx, db: db, stmtMap: make(map[string]*sql.Stmt), insMap: make(map[string]*sql.Stmt),
		autoIdMap: make(map[string]*sql.Stmt), updMap: make(map[string]*sql.Stmt), delMap: make(map[string]*sql.Stmt),
		sequences: sequences, datasourceConfig: datasourceConfig}
	return &trx
}

func Execute(config sql2.DatasourceConfig, callback func(trx *Transaction, args ...interface{}) interface{}, args ...interface{}) interface{} {
	db := sql2.GlobalDatabases.OpenDB(config)
	return doExecute(config, db, callback, args)
}

func doExecute(config sql2.DatasourceConfig, db *sql.DB, callback func(trx *Transaction, args ...interface{}) interface{}, args []interface{}) interface{} {
	tx, err := db.Begin()
	util.CheckErr(err)
	defer sql2.RollbackOnPanic(tx)
	trx := NewTransaction(config, tx, db)
	r := callback(trx, args...)
	util.CheckErr(tx.Commit())
	if trx.future != nil {
		for _, f := range trx.future {
			f()
		}
	}
	return r
}

func ExecuteRO(config sql2.DatasourceConfig, callback func(trx *Transaction, args ...interface{}) interface{}, args ...interface{}) interface{} {
	db := sql2.GlobalDatabases.OpenDB(config)
	return doExecuteRO(config, db, callback, args)
}

func doExecuteRO(config sql2.DatasourceConfig, db *sql.DB, callback func(trx *Transaction, args ...interface{}) interface{}, args []interface{}) interface{} {
	ctx := context.TODO()
	opts := sql.TxOptions{ReadOnly: true, Isolation: sql.LevelReadCommitted}
	tx, err := db.BeginTx(ctx, &opts)
	util.CheckErr(err)
	defer sql2.RollbackOnPanic(tx)
	trx := NewTransaction(config, tx, db)
	r := callback(trx, args...)
	util.CheckErr(tx.Commit())
	return r
}

type ContextKey string

const TxCtxContextkey = ContextKey("txCtx")

func InterceptTransactional(datasourceConfig sql2.DatasourceConfig, delegate func(tx *Transaction, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		Execute(datasourceConfig, func(trx *Transaction, args ...interface{}) interface{} {
			ctx := context.WithValue(r.Context(), TxCtxContextkey, trx)
			delegate(trx, w, r.WithContext(ctx))
			return nil
		})
	}
}

func InterceptTransactionalRO(datasourceConfig sql2.DatasourceConfig, delegate func(tx *Transaction, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ExecuteRO(datasourceConfig, func(trx *Transaction, args ...interface{}) interface{} {
			ctx := context.WithValue(r.Context(), TxCtxContextkey, trx)
			delegate(trx, w, r.WithContext(ctx))
			return nil
		})
	}
}

type Connection struct {
	DatasourceConfig *sql2.DatasourceConfig
	Db               *sql.DB
}

func (o *Connection) Open() {
	db := sql2.GlobalDatabases.OpenDB(*o.DatasourceConfig)
	o.Db = db
}

func (o *Connection) Close() {
	sql2.CloseDB(o.Db)
	o.Db = nil
}

func (o *Connection) Execute(callback func(trx *Transaction, args ...interface{}) interface{}, args ...interface{}) interface{} {

	return doExecute(*o.DatasourceConfig, o.Db, callback, args)
}

func (o *Connection) ExecuteRO(callback func(trx *Transaction, args ...interface{}) interface{}, args ...interface{}) interface{} {
	return doExecuteRO(*o.DatasourceConfig, o.Db, callback, args)
}

func NewConnection(datasourceConfig sql2.DatasourceConfig) *Connection {
	return &Connection{DatasourceConfig: &datasourceConfig}
}
