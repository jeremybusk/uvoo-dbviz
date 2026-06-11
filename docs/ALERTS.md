# Alerts

## Condition Type Alert Example

In the UI:

1. Open **Alerts** in the left sidebar.
2. Click **New**.
3. Set **Rule** to a name, for example `Timeout logs`.
4. Make sure the current query is the query you want the alert to run.
   - For logs, use the **Query** section first to select `logs`.
   - You can use builder mode or SQL mode.
5. In **Evaluator**, choose **Text match**.
6. In **Condition**:
   - First field: column to inspect, usually `body`, `message`, or whichever column your query returns.
   - Operator: `contains`, `not contains`, `=`, `!=`, or `regex`.
   - Text value: for example `timeout`.
7. Set **For** if you want the match to persist before firing, like `5m`; otherwise leave blank.
8. Set **Interval**, **Enabled**, and **Contact**.
9. Click **Test**.
10. Click **Save rule**.

For a simple log text alert, use SQL mode with something like:

```sql
SELECT
  service_name,
  body AS message,
  value,
  concat(service_name, ':timeout') AS fingerprint
FROM (
  SELECT
    service_name,
    any(body) AS body,
    count() AS value
  FROM otel_logs
  WHERE tenant_id = {tenant:String}
    AND timestamp >= {from:DateTime}
    AND timestamp < {to:DateTime}
    AND positionCaseInsensitive(body, 'timeout') > 0
  GROUP BY service_name
)
```

Then in Alerts:
- **Evaluator:** `Text match`
- **Field:** `message`
- **Operator:** `contains`
- **Value:** `timeout`

For best results, return a `fingerprint` column so each service/error group gets its own incident instead of one shared alert.
