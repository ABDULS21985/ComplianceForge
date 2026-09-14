package observability

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// queueStorageCollector exposes bounded durable-delivery state. It deliberately
// reports only fixed status values and aggregate ages; queue, tenant, owner,
// message, and task identifiers never become labels.
type queueStorageCollector struct {
	pool           *pgxpool.Pool
	outboxMessages *prometheus.Desc
	inboxMessages  *prometheus.Desc
	oldestPending  *prometheus.Desc
	activeLeases   *prometheus.Desc
	scrapeSuccess  *prometheus.Desc
}

func (m *Metrics) RegisterQueueStorage(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	m.registry.MustRegister(&queueStorageCollector{
		pool:           pool,
		outboxMessages: prometheus.NewDesc("complianceforge_outbox_messages", "Durable outbox rows by fixed status.", []string{"status"}, nil),
		inboxMessages:  prometheus.NewDesc("complianceforge_inbox_messages", "Durable inbox rows by fixed status.", []string{"status"}, nil),
		oldestPending:  prometheus.NewDesc("complianceforge_outbox_oldest_pending_age_seconds", "Age of the oldest dispatchable outbox row.", nil, nil),
		activeLeases:   prometheus.NewDesc("complianceforge_scheduler_active_leases", "Number of unexpired distributed scheduler leases.", nil, nil),
		scrapeSuccess:  prometheus.NewDesc("complianceforge_queue_storage_scrape_success", "Whether durable queue state was collected successfully.", nil, nil),
	})
}

func (c *queueStorageCollector) Describe(output chan<- *prometheus.Desc) {
	output <- c.outboxMessages
	output <- c.inboxMessages
	output <- c.oldestPending
	output <- c.activeLeases
	output <- c.scrapeSuccess
}

func (c *queueStorageCollector) Collect(output chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	outbox := map[string]float64{"pending": 0, "leased": 0, "published": 0, "dead": 0}
	inbox := map[string]float64{"processing": 0, "completed": 0}
	rows, err := c.pool.Query(ctx, `SELECT status, COUNT(*) FROM queue_outbox GROUP BY status`)
	if err == nil {
		for rows.Next() {
			var status string
			var count int64
			if scanErr := rows.Scan(&status, &count); scanErr != nil {
				err = scanErr
				break
			}
			if _, allowed := outbox[status]; allowed {
				outbox[status] = float64(count)
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			err = rowsErr
		}
		rows.Close()
	}
	if err == nil {
		rows, err = c.pool.Query(ctx, `SELECT status, COUNT(*) FROM queue_inbox GROUP BY status`)
	}
	if err == nil {
		for rows.Next() {
			var status string
			var count int64
			if scanErr := rows.Scan(&status, &count); scanErr != nil {
				err = scanErr
				break
			}
			if _, allowed := inbox[status]; allowed {
				inbox[status] = float64(count)
			}
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			err = rowsErr
		}
		rows.Close()
	}
	var oldestPending float64
	var activeLeases int64
	if err == nil {
		err = c.pool.QueryRow(ctx, `
			SELECT
				COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at) FILTER (
					WHERE (status = 'pending' AND available_at <= NOW())
						OR (status = 'leased' AND leased_until <= NOW())
				))), 0),
				(SELECT COUNT(*) FROM scheduler_leases WHERE leased_until > NOW())
			FROM queue_outbox`).Scan(&oldestPending, &activeLeases)
	}

	if err != nil {
		output <- prometheus.MustNewConstMetric(c.scrapeSuccess, prometheus.GaugeValue, 0)
		return
	}
	if oldestPending < 0 {
		oldestPending = 0
	}
	for _, status := range []string{"pending", "leased", "published", "dead"} {
		output <- prometheus.MustNewConstMetric(c.outboxMessages, prometheus.GaugeValue, outbox[status], status)
	}
	for _, status := range []string{"processing", "completed"} {
		output <- prometheus.MustNewConstMetric(c.inboxMessages, prometheus.GaugeValue, inbox[status], status)
	}
	output <- prometheus.MustNewConstMetric(c.oldestPending, prometheus.GaugeValue, oldestPending)
	output <- prometheus.MustNewConstMetric(c.activeLeases, prometheus.GaugeValue, float64(activeLeases))
	output <- prometheus.MustNewConstMetric(c.scrapeSuccess, prometheus.GaugeValue, 1)
}
