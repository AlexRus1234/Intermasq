# tests/suites/28-hosts-apply-template.sh — POST /api/hosts/apply-template.
# P3.8: this endpoint had no smoke coverage. applyTemplateHandler computes a
# free IP + hostname from a stored template (NO file side-effect — it returns
# the computed {mac, ip, hostname, file} only), so we create a template,
# apply it to a MAC, and assert 200 + a non-empty ip field.

if require_jwt "hosts apply-template" 3; then
    TPL_ID="apply-e2e"
    TPL_FILE="$CONF_DIR/e2e-apply.conf"

    # Clean up a leftover template from a prior run (404 is fine — ignored).
    DELETE "$JWT" "/api/templates/$TPL_ID" >/dev/null 2>&1 || true

    # Create the template the apply endpoint will resolve. ID is derived from
    # Name via lowercase + space→hyphen ("Apply E2E" → "apply-e2e").
    S=$(POST "$JWT" "/api/templates" "{\"name\":\"Apply E2E\",\"hostname_pattern\":\"apply-{NNN}\",\"ip_range\":\"10.99.0.0/24\",\"target_file\":\"$TPL_FILE\"}")
    check "Create template for apply-template → 200" 200 "$S" || true

    # Apply it to a fresh MAC: returns the computed {mac, ip, hostname, file}.
    S=$(POST "$JWT" "/api/hosts/apply-template" "{\"mac\":\"aa:dd:00:00:00:01\",\"template_id\":\"$TPL_ID\"}")
    check "POST /api/hosts/apply-template → 200" 200 "$S" || true

    APPLY_IP=$(body | jval .ip)
    echo "  apply-template ip: $APPLY_IP"
    if [ -n "$APPLY_IP" ] && [ "$APPLY_IP" != "null" ]; then
        check "apply-template returns non-empty ip" 1 1 || true
    else
        check "apply-template returns non-empty ip" 1 0 || true
    fi

    # Leave the templates store tidy for the next run.
    DELETE "$JWT" "/api/templates/$TPL_ID" >/dev/null 2>&1 || true
fi
