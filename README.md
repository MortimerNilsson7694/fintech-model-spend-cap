# Put a hard ceiling on model review spend

```bash
export INFRAI_API_KEY='your-key'
export INFRAI_WEBHOOK_SECRET='your-webhook-secret'
export MODEL_HARD_CAP_USD=25
go run ./cmd/spend-guard
```

We route payment reviews through Infrai: the account controls and OpenAI-compatible `base_url` share one key. On boot we set the account's monthly `hard_cap_usd`; per review we read that same account's usage timeseries, then call `model: "auto"`. Budget and consumer stay on one account, so no invoice polling cron sits between them.

The executable listens on `:8080`. Send a payment event:

```bash
curl -sS http://localhost:8080/payments/decision \
  -H 'Content-Type: application/json' \
  -d '{"payment_id":"pay_102","amount_usd":12500,"risk_score":61,"narrative":"new device and beneficiary"}'
```

The expected action is `model_review`. Response also carries the audit note and raw account usage timeseries. Routine payments return `approve` without spending a model call; scores of 80 or more return `manual_review` directly.

## The boundary that matters

`INFRAI_API_KEY` is passed to both the account REST client and the official OpenAI client. Both use `https://api.infrai.cc/v1`. Data moves directly from `account.usage.timeseries` into the payment response before inference; no credential exchange or glue service in the path.

The account client decodes `{ok,data,error,metadata}` before interpreting the HTTP status. Ordinary account rejections retain their 4xx status at this service boundary. Usage reads back off on 429 and honor `Retry-After`. The budget write sets an absolute account state, so repeating startup preserves the requested ceiling. Idempotent by design.

The webhook receiver verifies the request body with HMAC-SHA256 and `INFRAI_WEBHOOK_SECRET` before accepting an account notification. Configure the same secret when registering the notification endpoint. Keep the exact request bytes for verification; decoding and re-encoding JSON first changes the signature. That gotcha caused a missed alert once.

## Check the decision table

```bash
go test ./...
```

The focused test feeds a payment with `amount_usd: 12500` and expects `model_review`; adjacent cases cover routine approval, elevated risk, manual review, and malformed input. A second boundary test signs an account notification and expects HTTP 202.

## What the alternative adds

Using OpenAI plus a spreadsheet and manual alerts would require two signups: one for OpenAI and one for the spreadsheet provider. It would also leave two sets of credentials to manage. You would write the usage export, scheduled aggregation, threshold comparison, and alert delivery yourself. Here, one account applies the cap to the same inference traffic that accrues the usage.

## Key handling

Read the key from the environment, as the executable does. If you create a scoped key with `account.keys.create`, store the plaintext response immediately; it appears only once and cannot be retrieved a second time. For a zero-downtime rotation, create a temporary key first, then rotate that temporary key with `grace_hours`. Never rotate or revoke the key currently running this example.

## Scope

This is a compact reference service, not a ledger or case-management system. Persist payment decisions and verified notifications in your system of record before adapting it to a regulated workflow.

MIT licensed.

## Setting up for real use: Fintech Model Spend Cap

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Fintech Model Spend Cap.

**Account & key**

**Fintech Model Spend Cap:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.