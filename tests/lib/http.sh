# tests/lib/http.sh — curl wrappers + JSON helpers.
#
# All wrappers write the response body to /tmp/smoke.body and print the
# HTTP status code on stdout. body() reads the last written body.
# Depends on lib/state.sh (BASE, SECRET).

# jq-like helper: extract JSON field from stdin (requires jq)
jval() { jq -r "$1" 2>/dev/null; }

# HTTP request helpers (status code on stdout, body to /tmp/smoke.body)
GET()    { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $1" "$BASE$2"; }
POST()   { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $1" -H "Content-Type: application/json" -X POST -d "$3" "$BASE$2"; }
DELETE() { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Authorization: Bearer $1" -X DELETE "$BASE$2"; }
PGET()   { curl -s -o /tmp/smoke.body -w "%{http_code}" "$BASE$1"; }                # no-auth GET
PPOST()  { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "Content-Type: application/json" -X POST -d "$2" "$BASE$1"; }  # no-auth POST
KGET()   { curl -s -o /tmp/smoke.body -w "%{http_code}" -H "X-API-Key: $1" "$BASE$2"; }  # api-key GET

body() { cat /tmp/smoke.body; }
