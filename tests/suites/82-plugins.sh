# tests/suites/82-plugins.sh — GET /api/plugins + plugin proxy (Gap 6).
#
# The CI workflow installs the mock "hello" plugin into
# /etc/intermasq/plugins/hello/ before starting intermasq, so loadPlugins()
# picks it up. Here we verify both that intermasq lists it and that requests
# to /plugins/hello/* are reverse-proxied onto the plugin's unix socket.

if require_jwt "plugins" 3; then
    S=$(GET "$JWT" "/api/plugins")
    check "GET /api/plugins → 200" 200 "$S" || true
    echo "  body: $(body)"

    # Gap 6: the hello fixture must show up in the loaded-plugins list.
    if body | jval 'map(.id)' | grep -q '"hello"'; then
        check "hello plugin is loaded" 0 0 || true
    else
        check "hello plugin is loaded" 0 1 || true
    fi

    # Gap 6: /plugins/hello/health is proxied onto the plugin's socket.
    # Plugin routes are protected by the standard auth middleware.
    S=$(GET "$JWT" "/plugins/hello/health")
    check "GET /plugins/hello/health proxied → 200" 200 "$S" || true
    echo "  proxy body: $(body)"
fi
