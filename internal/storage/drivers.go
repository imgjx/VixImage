package storage

// Blank-import the SQL drivers so they self-register with database/sql.
// modernc.org/sqlite is pure-Go (no cgo); go-sql-driver/mysql is the MySQL driver.
import (
	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)
