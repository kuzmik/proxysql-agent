package proxysql

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/DATA-DOG/go-sqlmock.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testFixture struct {
	db    *sql.DB
	mock  sqlmock.Sqlmock
	proxy *ProxySQL
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock database: %v", err)
	}

	mock.MatchExpectationsInOrder(true)

	return &testFixture{
		db:    db,
		mock:  mock,
		proxy: &ProxySQL{db, tmpConfig, nil},
	}
}

func (f *testFixture) cleanup(t *testing.T) {
	t.Helper()

	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}

	f.db.Close()
}

func (f *testFixture) expectRuntimeCommands() {
	commands := []string{
		"LOAD PROXYSQL SERVERS TO RUNTIME",
		"LOAD ADMIN VARIABLES TO RUNTIME",
		"LOAD MYSQL VARIABLES TO RUNTIME",
		"LOAD MYSQL SERVERS TO RUNTIME",
		"LOAD MYSQL USERS TO RUNTIME",
		"LOAD MYSQL QUERY RULES TO RUNTIME",
	}
	for _, cmd := range commands {
		f.mock.ExpectExec(cmd).WillReturnResult(sqlmock.NewResult(0, 1))
	}
}

func newTestPod(name, ip, component string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
			Labels:    map[string]string{"component": component},
		},
		Status: v1.PodStatus{PodIP: ip},
	}
}

func TestPodUpdated(t *testing.T) {
	tests := []struct {
		name     string
		oldPhase v1.PodPhase
		newPhase v1.PodPhase
		setup    func(*testFixture)
	}{
		{
			name:     "pod started",
			oldPhase: v1.PodPending,
			newPhase: v1.PodRunning,
			setup: func(f *testFixture) {
				f.mock.ExpectExec("DELETE FROM proxysql_servers WHERE hostname = 'proxysql-core'").
					WillReturnResult(sqlmock.NewResult(0, 1))
				f.mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO proxysql_servers VALUES ("new-pod-ip", 6032, 0, "new-pod")`)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				f.expectRuntimeCommands()
			},
		},
		{
			name:     "pod stopped",
			oldPhase: v1.PodRunning,
			newPhase: v1.PodFailed,
			setup: func(f *testFixture) {
				f.mock.ExpectExec(`DELETE FROM proxysql_servers WHERE hostname = "old-pod-ip"`).
					WillReturnResult(sqlmock.NewResult(0, 1))
				f.expectRuntimeCommands()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFixture(t)
			defer f.cleanup(t)

			oldPod := newTestPod("old-pod", "old-pod-ip", "core")
			oldPod.Status.Phase = tt.oldPhase

			newPod := newTestPod("new-pod", "new-pod-ip", "core")
			newPod.Status.Phase = tt.newPhase

			tt.setup(f)
			f.proxy.podUpdated(oldPod, newPod)
		})
	}
}

func TestPodAdded(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("failed to get hostname: %v", err)
	}

	tests := []struct {
		name  string
		count int
		setup func(*testFixture)
	}{
		{
			name:  "core pod already exists in cluster",
			count: 1,
			setup: func(f *testFixture) {
				f.mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM proxysql_servers WHERE hostname = "pod-ip"`)).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			},
		},
		{
			name:  "core pod does not exist in cluster",
			count: 0,
			setup: func(f *testFixture) {
				f.mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM proxysql_servers WHERE hostname = "pod-ip"`)).
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				f.mock.ExpectExec("DELETE FROM proxysql_servers WHERE hostname = 'proxysql-core'").
					WillReturnResult(sqlmock.NewResult(0, 1))
				f.mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(`INSERT INTO proxysql_servers VALUES ("pod-ip", 6032, 0, %q)`, hostname))).
					WillReturnResult(sqlmock.NewResult(0, 1))
				f.expectRuntimeCommands()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFixture(t)
			defer f.cleanup(t)

			pod := newTestPod(hostname, "pod-ip", "core")

			tt.setup(f)
			f.proxy.podAdded(pod)
		})
	}
}

func TestRemovePodFromCluster(t *testing.T) {
	tests := []struct {
		name      string
		component string
		setup     func(*testFixture)
	}{
		{
			name:      "core pod",
			component: "core",
			setup: func(f *testFixture) {
				f.mock.ExpectExec(`DELETE FROM proxysql_servers WHERE hostname = "pod-ip"`).
					WillReturnResult(sqlmock.NewResult(0, 1))
				f.expectRuntimeCommands()
			},
		},
		{
			name:      "satellite pod",
			component: "satellite",
			setup: func(f *testFixture) {
				f.expectRuntimeCommands()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFixture(t)
			defer f.cleanup(t)

			pod := newTestPod("test-pod", "pod-ip", tt.component)
			tt.setup(f)

			if err := f.proxy.removePodFromCluster(pod); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
