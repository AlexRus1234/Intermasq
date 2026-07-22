# tests/suites/42-templates-hosts.sh — host templates CRUD + ranges list.
# Validates: list, create, duplicate-create, missing fields, delete,
# delete-missing, ranges endpoint.
# NOTE: ID is derived from Name via lowercase + space→hyphen replace, so
# "IoT devs" → "iot-devs".

if require_jwt "host templates + ranges" 7; then
    # List templates (initial state — likely empty in CI).
    S=$(GET "$JWT" "/api/templates")
    check "GET /api/templates → 200" 200 "$S" || true

    # Create a template.
    S=$(POST "$JWT" "/api/templates" "{\"name\":\"IoT devs\",\"hostname_pattern\":\"iot-{NNN}\",\"ip_range\":\"10.99.0.0/24\",\"target_file\":\"$FILE\"}")
    check "Create template → 200" 200 "$S" || true

    # Create the same template again — derived ID collides.
    S=$(POST "$JWT" "/api/templates" "{\"name\":\"IoT devs\",\"hostname_pattern\":\"iot-{NNN}\",\"ip_range\":\"10.99.0.0/24\",\"target_file\":\"$FILE\"}")
    check "Create template again → 409" 409 "$S" || true

    # Missing required fields.
    S=$(POST "$JWT" "/api/templates" "{\"name\":\"Empty\"}")
    check "Create template missing fields → 400" 400 "$S" || true

    # Delete the created template (id = "iot-devs").
    S=$(DELETE "$JWT" "/api/templates/iot-devs")
    check "Delete template → 200" 200 "$S" || true

    # Delete again — should be 404 now.
    S=$(DELETE "$JWT" "/api/templates/iot-devs")
    check "Delete missing template → 404" 404 "$S" || true

    # Ranges list — depends on dhcp-range directives present in config files.
    # In CI it may be empty; we only assert 200.
    S=$(GET "$JWT" "/api/templates/ranges")
    check "GET /api/templates/ranges → 200" 200 "$S" || true
fi
