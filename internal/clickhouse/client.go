package clickhouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"uvoo-dbviz/internal/config"
)

type Client struct {
	cfg  config.ClickHouseConfig
	http *http.Client
}

func NewClient(cfg config.ClickHouseConfig, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	if client.Timeout == 0 {
		client.Timeout = cfg.Timeout
	}
	return &Client{cfg: cfg, http: client}
}

func (c *Client) QueryJSONEachRow(ctx context.Context, sql string) ([]map[string]any, error) {
	endpoint, err := url.Parse(c.cfg.URL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("database", c.cfg.Database)
	query.Set("max_result_rows", fmt.Sprint(c.cfg.MaxRows))
	query.Set("max_execution_time", fmt.Sprint(c.cfg.MaxQuerySeconds))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewBufferString(sql))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if c.cfg.User != "" {
		req.SetBasicAuth(c.cfg.User, c.cfg.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("clickhouse returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	rows := []map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	for {
		var row map[string]any
		if err := decoder.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 && strings.TrimSpace(string(body)) != "" {
		for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
			var row map[string]any
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

type QueryRequest struct {
	Dataset       string            `json:"dataset"`
	From          time.Time         `json:"from"`
	To            time.Time         `json:"to"`
	GroupBy       string            `json:"groupBy"`
	Filters       map[string]string `json:"filters"`
	BucketSeconds int               `json:"bucketSeconds"`
	Limit         int               `json:"limit"`
}

func BuildTimeseriesSQL(req QueryRequest, ds config.Dataset, tenantID string, maxRows int) (string, error) {
	if req.Dataset == "" {
		return "", errors.New("dataset is required")
	}
	if tenantID == "" {
		return "", errors.New("tenant is required")
	}
	if req.To.IsZero() {
		req.To = time.Now()
	}
	if req.From.IsZero() {
		req.From = req.To.Add(-1 * time.Hour)
	}
	if !req.From.Before(req.To) {
		return "", errors.New("from must be before to")
	}
	if req.BucketSeconds <= 0 {
		req.BucketSeconds = bucketForRange(req.To.Sub(req.From))
	}
	if req.Limit <= 0 || req.Limit > maxRows {
		req.Limit = maxRows
	}
	if err := validateIdentifier(ds.Table, ds.TimeColumn, ds.TenantColumn); err != nil {
		return "", err
	}
	groupExpr := "'all'"
	groupAlias := "series"
	if req.GroupBy != "" {
		if !contains(ds.Dimensions, req.GroupBy) {
			return "", fmt.Errorf("groupBy is not allowed for dataset: %s", req.GroupBy)
		}
		if err := validateIdentifier(req.GroupBy); err != nil {
			return "", err
		}
		groupExpr = ident(req.GroupBy)
	}

	where := []string{
		fmt.Sprintf("%s >= parseDateTimeBestEffort(%s)", ident(ds.TimeColumn), quote(req.From.UTC().Format(time.RFC3339))),
		fmt.Sprintf("%s < parseDateTimeBestEffort(%s)", ident(ds.TimeColumn), quote(req.To.UTC().Format(time.RFC3339))),
		fmt.Sprintf("%s = %s", ident(ds.TenantColumn), quote(tenantID)),
	}
	for key, value := range req.Filters {
		if value == "" {
			continue
		}
		if !contains(ds.Filters, key) {
			return "", fmt.Errorf("filter is not allowed for dataset: %s", key)
		}
		if err := validateIdentifier(key); err != nil {
			return "", err
		}
		where = append(where, fmt.Sprintf("%s = %s", ident(key), quote(value)))
	}

	sql := fmt.Sprintf(`SELECT
  toUnixTimestamp(toStartOfInterval(%s, INTERVAL %d SECOND)) AS ts,
  toString(%s) AS %s,
  toFloat64(count()) AS value
FROM %s
WHERE %s
GROUP BY ts, %s
ORDER BY ts ASC
LIMIT %d
FORMAT JSONEachRow`,
		ident(ds.TimeColumn),
		req.BucketSeconds,
		groupExpr,
		groupAlias,
		ident(ds.Table),
		strings.Join(where, " AND "),
		groupAlias,
		req.Limit,
	)
	return sql, nil
}

func bucketForRange(d time.Duration) int {
	switch {
	case d <= time.Hour:
		return 60
	case d <= 24*time.Hour:
		return 300
	case d <= 7*24*time.Hour:
		return 3600
	default:
		return 21600
	}
}

func validateIdentifier(values ...string) error {
	for _, value := range values {
		if value == "" {
			return errors.New("identifier cannot be empty")
		}
		for _, part := range strings.Split(value, ".") {
			if part == "" {
				return fmt.Errorf("invalid identifier: %s", value)
			}
			for i, r := range part {
				if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9' {
					continue
				}
				return fmt.Errorf("invalid identifier: %s", value)
			}
		}
	}
	return nil
}

func ident(value string) string {
	parts := strings.Split(value, ".")
	for i, part := range parts {
		parts[i] = "`" + strings.ReplaceAll(part, "`", "``") + "`"
	}
	return strings.Join(parts, ".")
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
