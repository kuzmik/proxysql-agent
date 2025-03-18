package proxysql

import (
	"errors"
	"reflect"
	"testing"

	"github.com/persona-id/proxysql-agent/internal/configuration"

	"gopkg.in/DATA-DOG/go-sqlmock.v2"
)

//nolint:gochecknoglobals
var tmpConfig = &configuration.Config{
	StartDelay: 0,
	Log: struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
	}{},
	ProxySQL: struct {
		Address  string `mapstructure:"address"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}{},
	RunMode: "",
	Core: struct {
		Interval    int `mapstructure:"interval"`
		PodSelector struct {
			Namespace string `mapstructure:"namespace"`
			App       string `mapstructure:"app"`
			Component string `mapstructure:"component"`
		} `mapstructure:"podselector"`
	}{},
	Satellite: struct {
		Interval int `mapstructure:"interval"`
	}{},
	Interfaces: []string{},
}

func TestProxySQL(t *testing.T) {
	t.Run("ping", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock database: %v", err)
		}
		defer db.Close()

		proxy := &ProxySQL{db, tmpConfig, nil}
		if err := proxy.Ping(); err != nil {
			t.Errorf("Ping() returned unexpected error: %v", err)
		}

		if proxy.conn == nil {
			t.Error("expected conn to not be nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("SQL expectations were not met: %v", err)
		}
	})

	t.Run("get backends", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock database: %v", err)
		}
		defer db.Close()

		proxy := &ProxySQL{db, tmpConfig, nil}

		t.Run("success", func(t *testing.T) {
			expectedRows := sqlmock.NewRows([]string{"hostgroup_id", "hostname", "port"}).
				AddRow(1, "host1", 3306).
				AddRow(2, "host2", 3306).
				AddRow(1, "host3", 3307)

			mock.ExpectQuery("SELECT hostgroup_id, hostname, port FROM runtime_mysql_servers ORDER BY hostgroup_id").
				WillReturnRows(expectedRows)

			entries, err := proxy.GetBackends()
			if err != nil {
				t.Errorf("GetBackends() returned unexpected error: %v", err)
			}

			expectedEntries := map[string]int{
				"host1": 1,
				"host2": 2,
				"host3": 1,
			}

			if !reflect.DeepEqual(entries, expectedEntries) {
				t.Errorf("got entries = %v, want %v", entries, expectedEntries)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("SQL expectations were not met: %v", err)
			}
		})

		t.Run("database error", func(t *testing.T) {
			expectedError := errors.New("database error")
			mock.ExpectQuery("SELECT hostgroup_id, hostname, port FROM runtime_mysql_servers ORDER BY hostgroup_id").
				WillReturnError(expectedError)

			_, err = proxy.GetBackends()
			if err == nil {
				t.Error("GetBackends() expected error, got nil")
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
}
