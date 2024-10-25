package coverage

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"

	sql2 "sparrowhawktech/toolkit/sql"
	"sparrowhawktech/toolkit/tx"
	"sparrowhawktech/toolkit/util"
)

type Config struct {
	DatasourceConfig                 *sql2.DatasourceConfig `json:"datasourceConfig"`
	SqlFolder                        *string                `json:"sqlFolder"`
	InitScripts                      []string               `json:"initScripts"`
	ApplicationPullIntervalInSeconds *int                   `json:"applicationPullIntervalInSeconds"`
	PatchesFile                      *string                `json:"patchesFile"`
}

func SetupDb(config Config, dbName string, callback func(txCtx *tx.Transaction)) {
	connString := fmt.Sprintf(*config.DatasourceConfig.Name, dbName)
	config.DatasourceConfig.Name = &connString
	noDbConfig := *config.DatasourceConfig
	noDbConnString, dbName := parseConnectionString(connString)
	noDbConfig.Name = &noDbConnString

	ExecuteDB(noDbConfig, func(db *sql.DB) {
		util.Log("info").Printf("Dropping %s", dbName)
		_, err := db.Exec("drop database if exists \"" + dbName + "\"")
		util.CheckErr(err)
	})

	ExecuteDB(noDbConfig, func(db *sql.DB) {
		util.Log("info").Printf("Creating %s", dbName)
		_, err := db.Exec("create database \"" + dbName + "\"")
		util.CheckErr(err)
	})

	for _, spec := range config.InitScripts {
		util.Log("info").Printf("Executing %s", spec)
		util.RunCmd("psql", *config.DatasourceConfig.Name, "-a", "-f", *config.SqlFolder+"/"+spec)
	}

	if config.PatchesFile != nil && util.FileExists(*config.PatchesFile) {
		processSqlPatches(config)
	}

	tx.Execute(*config.DatasourceConfig, func(trx *tx.Transaction, args ...interface{}) interface{} {
		callback(trx)
		return nil
	})
}

func processSqlPatches(config Config) {

	patchesToApplyFile, err := os.OpenFile(*config.PatchesFile, os.O_RDONLY, os.ModePerm)
	util.CheckErr(err)
	defer closeFile(patchesToApplyFile)

	sc := bufio.NewScanner(patchesToApplyFile)
	for sc.Scan() {
		patchName := sc.Text()
		if len(strings.TrimSpace(patchName)) == 0 {
			continue
		}
		util.Log("info").Printf("Executing %s", patchName)
		util.RunCmd("psql", *config.DatasourceConfig.Name, "-a", "-f", *config.SqlFolder+"/"+patchName)
	}
}

func parseConnectionString(connStr string) (string, string) {
	params := strings.Split(connStr, "?")[1]
	conn := strings.Split(strings.Split(connStr, "?")[0], "/")
	dbName := conn[3]
	noDbConnString := strings.Join(conn[:3], "/") + "/?" + params

	return noDbConnString, dbName
}

func closeFile(file *os.File) {
	err := file.Close()
	util.CheckErr(err)
}

func ExecuteDB(config sql2.DatasourceConfig, delegate func(db *sql.DB)) {
	db := sql2.OpenDB(config)
	defer sql2.CloseDB(db)
	delegate(db)
}
