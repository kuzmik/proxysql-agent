package proxysql

import (
	"errors"
	"regexp"
	"testing"

	"gopkg.in/DATA-DOG/go-sqlmock.v2"
)

func TestSatelliteOperations(t *testing.T) {
	t.Run("get missing core pods", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock database: %v", err)
		}
		defer db.Close()

		query := regexp.QuoteMeta("SELECT COUNT(hostname) FROM stats_proxysql_servers_metrics WHERE last_check_ms > 30000 AND hostname != 'proxysql-core' AND Uptime_s > 0")
		proxy := &ProxySQL{db, tmpConfig, nil}

		t.Run("success", func(t *testing.T) {
			expectedCount := 1
			mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(expectedCount))

			count, err := proxy.GetMissingCorePods()
			if err != nil {
				t.Errorf("GetMissingCorePods() returned unexpected error: %v", err)
			}

			if count != expectedCount {
				t.Errorf("got count = %d, want %d", count, expectedCount)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("SQL expectations were not met: %v", err)
			}
		})

		t.Run("database error", func(t *testing.T) {
			mock.ExpectQuery(query).WillReturnError(errors.New("database error"))

			count, err := proxy.GetMissingCorePods()
			if count != -1 {
				t.Errorf("got count = %d, want -1", count)
			}

			if err == nil {
				t.Error("expected error, got nil")
			}

			want := "failed to execute query: database error"
			if err.Error() != want {
				t.Errorf("got error = %q, want %q", err.Error(), want)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("SQL expectations were not met: %v", err)
			}
		})
	})

	t.Run("satellite resync", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock database: %v", err)
		}
		defer db.Close()

		mock.MatchExpectationsInOrder(true)

		proxy := &ProxySQL{conn: db}

		query := regexp.QuoteMeta("SELECT COUNT(hostname) FROM stats_proxysql_servers_metrics WHERE last_check_ms > 30000 AND hostname != 'proxysql-core' AND Uptime_s > 0")
		mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

		commands := []string{
			"DELETE FROM proxysql_servers",
			"LOAD PROXYSQL SERVERS FROM CONFIG",
			"LOAD PROXYSQL SERVERS TO RUNTIME;",
		}
		for _, command := range commands {
			mock.ExpectExec(command).WillReturnResult(sqlmock.NewResult(1, 1))
		}

		if err := proxy.SatelliteResync(); err != nil {
			t.Errorf("SatelliteResync() returned unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("SQL expectations were not met: %v", err)
		}
	})
}
