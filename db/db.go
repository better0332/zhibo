package db

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
)

var (
	db         *sql.DB
	stmtUpdate *sql.Stmt
	sqlUpdate  = `update live_proxy set online=?, cache_size=?, service_size=? where id=? and status=1`
)

func init() {
	var err error

	db, err = sql.Open("mysql", "root:@unix(/var/lib/mysql/mysql.sock)/cache?charset=utf8")
	if err != nil {
		glog.Fatal(err)
	}
	// db.SetMaxIdleConns(5)
	if stmtUpdate, err = db.Prepare(sqlUpdate); err != nil {
		glog.V(0).Infoln(err)
	}
}

func Update(liveId int, online, cacheSize, serviceSize int64) {
	if stmtUpdate == nil {
		var err error
		if stmtUpdate, err = db.Prepare(sqlUpdate); err != nil {
			glog.V(0).Infoln(err)
			return
		}
	}
	stmtUpdate.Exec(online, cacheSize, serviceSize, liveId)
	return
}

func QueryLiveId() (*sql.Rows, error) {
	sqlQuery := `select id, category, cache_size, service_size from live_proxy where status=1`
	return db.Query(sqlQuery)
}
