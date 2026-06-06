#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TARGET_TOTAL="${TARGET_TOTAL:-200}"

echo "==> Seeding RAG eval cases (target=${TARGET_TOTAL} published + negative cases)..."
SEED_JSON="$(curl -sS -X POST "${BASE_URL}/api/customer-service/admin/rag-eval-cases/seed?target_total=${TARGET_TOTAL}")"

echo "==> Running RAG regression..."
RUN_JSON="$(curl -sS -X POST "${BASE_URL}/api/customer-service/admin/rag-eval-cases/run")"

python3 - "$SEED_JSON" "$RUN_JSON" <<'PY'
import json
import sys

seed = json.loads(sys.argv[1])
run = json.loads(sys.argv[2])
quality = run.get("quality_summary", {})

print("Seed summary:")
print(f"  target_total: {seed.get('target_total')}")
print(f"  before_total: {seed.get('before_total')}")
print(f"  created: {seed.get('created')}")
print(f"  negative_created: {seed.get('negative_created')}")
print(f"  after_total: {seed.get('after_total')}")
print()
print("Run summary:")
print(f"  total: {run.get('total')}")
print(f"  passed: {run.get('passed')}")
print(f"  failed: {run.get('failed')}")
print(f"  pass_rate: {round((run.get('pass_rate') or 0) * 100, 2)}%")
print(f"  duration_ms: {run.get('duration_ms')}")
print(f"  top1_hit_rate: {round((quality.get('top1_hit_rate') or 0) * 100, 2)}%")
print(f"  top3_hit_rate: {round((quality.get('top3_hit_rate') or 0) * 100, 2)}%")
print(f"  should_not_answer_hit_count: {quality.get('should_not_answer_hit_count')}")
print(f"  suspected_hallucination: {quality.get('suspected_hallucination')}")
print(f"  major_unsafe_rate: {round((quality.get('major_unsafe_rate') or 0) * 100, 2)}%")

failed_items = [item for item in run.get("items", []) if not item.get("passed")]
if failed_items:
    print()
    print("Failed cases:")
    for item in failed_items[:20]:
        print(
            f"  - {item.get('case_id')}: {item.get('query_text')} "
            f"({item.get('reason')})"
        )
    if len(failed_items) > 20:
        print(f"  ... and {len(failed_items) - 20} more")

if run.get("failed", 0):
    sys.exit(1)
PY
