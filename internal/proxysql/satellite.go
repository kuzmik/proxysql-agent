package proxysql

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

//
// Satellite mode specific functions
//

func (p *ProxySQL) Satellite() {
	interval := p.settings.Satellite.Interval

	slog.Info("Satellite mode initialized, looping", slog.Int("interval", interval))

	for {
		err := p.SatelliteResync()
		if err != nil {
			slog.Error("Error running resync", slog.Any("error", err))
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func (p *ProxySQL) GetMissingCorePods() (int, error) {
	count := -1

	query := `SELECT COUNT(hostname)
			FROM stats_proxysql_servers_metrics
			WHERE last_check_ms > 30000
			AND hostname != 'proxysql-core'
			AND Uptime_s > 0`
	row := p.conn.QueryRow(query)

	err := row.Scan(&count)
	if err != nil {
		return count, fmt.Errorf("failed to execute query: %w", err)
	}

	return count, nil
}

func (p *ProxySQL) SatelliteResync() error {
	missing, err := p.GetMissingCorePods()
	if err != nil {
		return fmt.Errorf("failed to get missing core pods: %w", err)
	}

	if missing > 0 {
		slog.Info("Resyncing pod to cluster", slog.Int("missing_cores", missing))

		commands := []string{
			"DELETE FROM proxysql_servers",
			"LOAD PROXYSQL SERVERS FROM CONFIG",
			"LOAD PROXYSQL SERVERS TO RUNTIME;",
		}

		for _, command := range commands {
			_, err := p.conn.Exec(command)
			if err != nil {
				return fmt.Errorf("failed to execute command %s: %w", command, err)
			}
		}
	}

	return nil
}

// data we eventually want to load into snowflake
//  1. stats_mysql_query_digests (maybe use _reset to reset the state)
//  2. mysql_query_rules
//  3. stats_mysql_query_rules
//
// FIXME: all these functions dump to /tmp/XXXX/Y.csv; we want the directory to be configurable at least.
func (p *ProxySQL) DumpData() {
	tmpdir, _ := os.MkdirTemp("/tmp", "")

	digestsFile, err := p.DumpQueryDigests(tmpdir)
	if err != nil {
		slog.Error("Error in DumpQueryDigests()", slog.Any("error", err))
	} else if digestsFile != "" {
		slog.Info("Saved mysql query digests to file", slog.String("filename", digestsFile))
	}
}

// ProxySQL docs: https://proxysql.com/documentation/stats-statistics/#stats_mysql_query_digest
func (p *ProxySQL) DumpQueryDigests(tmpdir string) (string, error) {
	var rowCount int

	err := p.conn.QueryRow("SELECT COUNT(*) FROM stats_mysql_query_digest").Scan(&rowCount)
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}

	// Don't proceed with this function if there are no entries in the table
	if rowCount <= 0 {
		slog.Debug("No query digests in the log, not proceeding with DumpQueryDigests()")

		return "", nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}

	dumpFile := fmt.Sprintf("%s/%s-digests.csv", tmpdir, hostname)

	file, err := os.Create(dumpFile)
	if err != nil {
		return "", fmt.Errorf("failed to create file %s: %w", dumpFile, err)
	}

	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"pod_name",
		"hostgroup",
		"schemaname",
		"username",
		"digest",
		"digest_text",
		"count_star",
		"first_seen",
		"last_seen",
		"sum_time_us",
		"min_time_us",
		"max_time",
		"sum_rows_affected",
		"sum_rows_sent",
	}

	if err := writer.Write(header); err != nil {
		return "", fmt.Errorf("failed to write CSV header: %w", err)
	}

	rows, err := p.conn.Query("SELECT * FROM stats_mysql_query_digest")
	if err != nil {
		return "", fmt.Errorf("failed to execute query: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var hostgroup int

		var schemaname, username, clientAddress, digest, digestText string

		var countStar, firstSeen, lastSeen, sumTime, minTime, maxTime, sumRowsAffected, sumRowsSent int

		err := rows.Scan(&hostgroup, &schemaname, &username, &clientAddress, &digest, &digestText, &countStar,
			&firstSeen, &lastSeen, &sumTime, &minTime, &maxTime, &sumRowsAffected, &sumRowsSent)
		if err != nil {
			return "", fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a slice with the values
		values := []string{
			hostname,
			strconv.Itoa(hostgroup),
			schemaname,
			username,
			digest,
			`"` + digestText + `"`, // Quote the digest_text field to handle commas
			strconv.Itoa(countStar),
			time.Unix(int64(firstSeen), 0).String(),
			time.Unix(int64(lastSeen), 0).String(),
			strconv.Itoa(sumTime),
			strconv.Itoa(minTime),
			strconv.Itoa(maxTime),
			strconv.Itoa(sumRowsAffected),
			strconv.Itoa(sumRowsSent),
		}

		// Write the values to the CSV file
		if err := writer.Write(values); err != nil {
			return "", fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	return dumpFile, nil
}
