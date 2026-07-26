# tests/suites/41-config-put.sh — PUT /api/config (visual editor).
# Validates: valid directives, invalid directive key, newline in value,
# unsafe file path.
# writeConfigWithTest runs `dnsmasq --test --conf-file=<path>` (A13 fixed),
# so the "valid directives" case is genuinely validated against the written
# file; the other cases are rejected earlier (regex/escape/isSafePath).

if require_jwt "PUT /api/config (visual editor)" 5; then
    EDIT_FILE="$CONF_DIR/45-visual.conf"
    # Create the file first via the file-create endpoint (uses empty template).
    S=$(POST "$JWT" "/api/config/file" "{\"name\":\"45-visual.conf\",\"template\":\"empty\"}")
    check "Create 45-visual.conf for PUT /api/config" 200 "$S" || true

    # Happy: PUT a small set of valid boolean directives.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d "{\"file\":\"$EDIT_FILE\",\"directives\":[{\"key\":\"domain-needed\",\"value\":\"\",\"active\":true},{\"key\":\"bogus-priv\",\"value\":\"\",\"active\":true}]}" "$BASE/api/config")
    check "PUT /api/config valid directives → 200" 200 "$S" || true

    # Error: invalid directive key (must match ^[a-z][a-z0-9-]*$).
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d "{\"file\":\"$EDIT_FILE\",\"directives\":[{\"key\":\"123bad\",\"value\":\"\",\"active\":true}]}" "$BASE/api/config")
    check "PUT /api/config bad key → 400" 400 "$S" || true

    # Error: uppercase key.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d "{\"file\":\"$EDIT_FILE\",\"directives\":[{\"key\":\"DomainNeeded\",\"value\":\"\",\"active\":true}]}" "$BASE/api/config")
    check "PUT /api/config uppercase key → 400" 400 "$S" || true

    # Error: value with embedded newline.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d "{\"file\":\"$EDIT_FILE\",\"directives\":[{\"key\":\"server\",\"value\":\"1.1.1.1\\n8.8.8.8\",\"active\":true}]}" "$BASE/api/config")
    check "PUT /api/config newline value → 400" 400 "$S" || true

    # Error: unsafe file path.
    S=$(curl -s -o /tmp/smoke.body -w "%{http_code}" -X PUT -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d "{\"file\":\"/etc/evil.conf\",\"directives\":[]}" "$BASE/api/config")
    check "PUT /api/config unsafe file → 403" 403 "$S" || true
fi
