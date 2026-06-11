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
	"regexp"
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
	} else {
		copied := *client
		client = &copied
	}
	if client.Timeout == 0 {
		client.Timeout = cfg.Timeout
	}
	return &Client{cfg: cfg, http: client}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.QueryJSONEachRow(ctx, "SELECT 1 AS ok FORMAT JSONEachRow")
	return err
}

func (c *Client) QueryJSONEachRow(ctx context.Context, sql string) ([]map[string]any, error) {
	return c.QueryJSONEachRowWithParams(ctx, sql, nil)
}

func (c *Client) QueryJSONEachRowWithParams(ctx context.Context, sql string, params map[string]string) ([]map[string]any, error) {
	endpoint, err := url.Parse(c.cfg.URL)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("database", c.cfg.Database)
	query.Set("max_result_rows", fmt.Sprint(c.cfg.MaxRows))
	query.Set("max_execution_time", fmt.Sprint(c.cfg.MaxQuerySeconds))
	query.Set("readonly", "1")
	for key, value := range params {
		query.Set("param_"+key, value)
	}
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
	SourceID      string            `json:"sourceId"`
	Mode          string            `json:"mode"`
	SQL           string            `json:"sql"`
	From          time.Time         `json:"from"`
	To            time.Time         `json:"to"`
	GroupBy       string            `json:"groupBy"`
	Measure       string            `json:"measure"`
	Aggregation   string            `json:"aggregation"`
	Search        string            `json:"search"`
	Filters       map[string]string `json:"filters"`
	FilterOps     map[string]string `json:"filterOps"`
	BucketSeconds int               `json:"bucketSeconds"`
	Limit         int               `json:"limit"`
}

type CustomSQLMode string

const (
	CustomSQLExplore CustomSQLMode = "explore"
	CustomSQLAlert   CustomSQLMode = "alert"
)

type CustomSQLQuery struct {
	SQL    string
	Params map[string]string
}

var unsafeSQLTokens = regexp.MustCompile(`(?i)\b(INSERT|ALTER|DROP|DELETE|CREATE|TRUNCATE|OPTIMIZE|SYSTEM|ATTACH|DETACH|RENAME|GRANT|REVOKE|KILL|SET|USE|EXPLAIN|DESCRIBE|SHOW|WATCH|BACKUP|RESTORE|CHECK|CHECKSUM|FORMAT|SETTINGS)\b`)
var unsafeSQLFunctions = regexp.MustCompile(`(?i)\b(url|file|s3|hdfs|mysql|postgresql|remote|remoteSecure|cluster|jdbc|odbc|executable|generateRandom)\s*\(`)

func BuildTimeseriesSQL(req QueryRequest, ds config.Dataset, tenantID string, maxRows int) (string, error) {
	req, where, err := buildWhere(req, ds, tenantID, maxRows)
	if err != nil {
		return "", err
	}
	if req.BucketSeconds <= 0 {
		req.BucketSeconds = bucketForRange(req.To.Sub(req.From))
	}
	measureExpr, err := measureSQL(req.Measure, req.Aggregation, ds)
	if err != nil {
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

	sql := fmt.Sprintf(`SELECT
  toUnixTimestamp(toStartOfInterval(%s, INTERVAL %d SECOND)) AS ts,
  toString(%s) AS %s,
  %s AS value
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
		measureExpr,
		ident(ds.Table),
		strings.Join(where, " AND "),
		groupAlias,
		req.Limit,
	)
	return sql, nil
}

func BuildCustomSQL(req QueryRequest, ds config.Dataset, tenantID string, maxRows int, mode CustomSQLMode) (CustomSQLQuery, error) {
	req, _, err := buildWhere(req, ds, tenantID, maxRows)
	if err != nil {
		return CustomSQLQuery{}, err
	}
	raw := strings.TrimSpace(req.SQL)
	if raw == "" {
		return CustomSQLQuery{}, errors.New("sql is required")
	}
	if err := validateCustomSQLText(raw); err != nil {
		return CustomSQLQuery{}, err
	}
	if !strings.Contains(raw, "{tenant:String}") {
		return CustomSQLQuery{}, errors.New("custom sql must include {tenant:String}")
	}
	if !strings.Contains(raw, "{from:DateTime}") {
		return CustomSQLQuery{}, errors.New("custom sql must include {from:DateTime}")
	}
	if !strings.Contains(raw, "{to:DateTime}") {
		return CustomSQLQuery{}, errors.New("custom sql must include {to:DateTime}")
	}
	limit := req.Limit
	if limit <= 0 || limit > maxRows {
		limit = maxRows
	}
	if ds.MaxRows > 0 && limit > ds.MaxRows {
		limit = ds.MaxRows
	}
	sql := fmt.Sprintf(`SELECT *
FROM (
%s
)
LIMIT {limit:UInt64}
FORMAT JSONEachRow`, raw)
	return CustomSQLQuery{
		SQL: sql,
		Params: map[string]string{
			"tenant": tenantID,
			"from":   req.From.UTC().Format("2006-01-02 15:04:05"),
			"to":     req.To.UTC().Format("2006-01-02 15:04:05"),
			"limit":  fmt.Sprint(limit),
		},
	}, nil
}

func validateCustomSQLText(sql string) error {
	if strings.Contains(sql, "\x00") {
		return errors.New("sql contains invalid null byte")
	}
	if strings.Contains(sql, ";") {
		return errors.New("custom sql must be a single statement without semicolons")
	}
	if strings.Contains(sql, "--") || strings.Contains(sql, "/*") || strings.Contains(sql, "*/") {
		return errors.New("custom sql comments are not allowed")
	}
	upperTrimmed := strings.ToUpper(strings.TrimSpace(sql))
	if !strings.HasPrefix(upperTrimmed, "SELECT ") && !strings.HasPrefix(upperTrimmed, "SELECT\n") {
		return errors.New("custom sql must start with SELECT")
	}
	if unsafeSQLTokens.MatchString(sql) {
		return errors.New("custom sql contains a blocked statement or clause")
	}
	if unsafeSQLFunctions.MatchString(sql) {
		return errors.New("custom sql contains a blocked table function")
	}
	return nil
}

func BuildEventsSQL(req QueryRequest, ds config.Dataset, tenantID string, maxRows int) (string, error) {
	req, where, err := buildWhere(req, ds, tenantID, maxRows)
	if err != nil {
		return "", err
	}
	columns := ds.EventColumns
	if len(columns) == 0 {
		columns = append([]string{ds.TimeColumn}, ds.Dimensions...)
		columns = append(columns, ds.Filters...)
		columns = append(columns, ds.Measures...)
	}
	if err := validateIdentifier(columns...); err != nil {
		return "", err
	}
	selects := make([]string, 0, len(columns)+1)
	selects = append(selects, fmt.Sprintf("toUnixTimestamp(%s) AS ts", ident(ds.TimeColumn)))
	seen := map[string]bool{ds.TimeColumn: true}
	for _, column := range columns {
		if seen[column] {
			continue
		}
		seen[column] = true
		selects = append(selects, fmt.Sprintf("%s AS %s", ident(column), ident(column)))
	}
	sql := fmt.Sprintf(`SELECT
  %s
FROM %s
WHERE %s
ORDER BY %s DESC
LIMIT %d
FORMAT JSONEachRow`,
		strings.Join(selects, ",\n  "),
		ident(ds.Table),
		strings.Join(where, " AND "),
		ident(ds.TimeColumn),
		req.Limit,
	)
	return sql, nil
}

func buildWhere(req QueryRequest, ds config.Dataset, tenantID string, maxRows int) (QueryRequest, []string, error) {
	if req.Dataset == "" {
		return req, nil, errors.New("dataset is required")
	}
	if tenantID == "" {
		return req, nil, errors.New("tenant is required")
	}
	if req.To.IsZero() {
		req.To = time.Now()
	}
	if req.From.IsZero() {
		req.From = req.To.Add(-1 * time.Hour)
	}
	if !req.From.Before(req.To) {
		return req, nil, errors.New("from must be before to")
	}
	if ds.MaxLookbackHours > 0 && req.To.Sub(req.From) > time.Duration(ds.MaxLookbackHours)*time.Hour {
		return req, nil, fmt.Errorf("query range exceeds dataset max lookback of %d hours", ds.MaxLookbackHours)
	}
	if req.Limit <= 0 || req.Limit > maxRows {
		req.Limit = maxRows
	}
	if ds.MaxRows > 0 && req.Limit > ds.MaxRows {
		req.Limit = ds.MaxRows
	}
	if err := validateIdentifier(ds.Table, ds.TimeColumn, ds.TenantColumn); err != nil {
		return req, nil, err
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
			return req, nil, fmt.Errorf("filter is not allowed for dataset: %s", key)
		}
		if err := validateIdentifier(key); err != nil {
			return req, nil, err
		}
		op := "eq"
		if req.FilterOps != nil && req.FilterOps[key] != "" {
			op = req.FilterOps[key]
		}
		if !operatorAllowed(ds, key, op) {
			return req, nil, fmt.Errorf("filter operator is not allowed for %s: %s", key, op)
		}
		where = append(where, filterSQL(key, op, value))
	}
	if strings.TrimSpace(req.Search) != "" {
		if len(ds.SearchColumns) == 0 {
			return req, nil, errors.New("search is not enabled for dataset")
		}
		if err := validateIdentifier(ds.SearchColumns...); err != nil {
			return req, nil, err
		}
		search := make([]string, 0, len(ds.SearchColumns))
		for _, column := range ds.SearchColumns {
			search = append(search, fmt.Sprintf("positionCaseInsensitive(toString(%s), %s) > 0", ident(column), quote(strings.TrimSpace(req.Search))))
		}
		where = append(where, "("+strings.Join(search, " OR ")+")")
	}
	return req, where, nil
}

func measureSQL(measure, aggregation string, ds config.Dataset) (string, error) {
	if measure == "" {
		measure = ds.DefaultMeasure
	}
	if aggregation == "" {
		aggregation = ds.DefaultAggregation
	}
	if measure == "" {
		measure = "_rows"
	}
	if aggregation == "" {
		aggregation = "count"
	}
	if !contains(ds.Measures, measure) {
		return "", fmt.Errorf("measure is not allowed for dataset: %s", measure)
	}
	if !contains(ds.Aggregations, aggregation) {
		return "", fmt.Errorf("aggregation is not allowed for dataset: %s", aggregation)
	}
	if aggregation == "count" {
		return "toFloat64(count())", nil
	}
	if measure == "_rows" {
		return "", errors.New("row count measure only supports count aggregation")
	}
	if err := validateIdentifier(measure); err != nil {
		return "", err
	}
	switch aggregation {
	case "avg":
		return fmt.Sprintf("toFloat64(avg(%s))", ident(measure)), nil
	case "sum":
		return fmt.Sprintf("toFloat64(sum(%s))", ident(measure)), nil
	case "min":
		return fmt.Sprintf("toFloat64(min(%s))", ident(measure)), nil
	case "max":
		return fmt.Sprintf("toFloat64(max(%s))", ident(measure)), nil
	case "p95":
		return fmt.Sprintf("toFloat64(quantile(0.95)(%s))", ident(measure)), nil
	default:
		return "", fmt.Errorf("unsupported aggregation: %s", aggregation)
	}
}

func operatorAllowed(ds config.Dataset, key, op string) bool {
	if len(ds.FilterOperators) == 0 {
		return op == "eq"
	}
	return contains(ds.FilterOperators[key], op)
}

func filterSQL(key, op, value string) string {
	switch op {
	case "contains":
		return fmt.Sprintf("positionCaseInsensitive(toString(%s), %s) > 0", ident(key), quote(value))
	case "prefix":
		return fmt.Sprintf("startsWith(toString(%s), %s)", ident(key), quote(value))
	default:
		return fmt.Sprintf("%s = %s", ident(key), quote(value))
	}
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
