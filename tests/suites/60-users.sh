# tests/suites/60-users.sh — user CRUD, password change, cannot-delete-self.

if require_jwt "users" 12; then
    S=$(GET "$JWT" "/api/users")
    check "GET /api/users" 200 "$S" || true

    S=$(POST "$JWT" "/api/users" "{\"username\":\"alice\",\"password\":\"alicepass\"}")
    check "Create user alice" 200 "$S" || true

    S=$(POST "$JWT" "/api/users" "{\"username\":\"alice\",\"password\":\"alicepass\"}")
    check "Create alice again → 409" 409 "$S" || true

    S=$(POST "$JWT" "/api/users" "{\"username\":\"$(printf 'x%.0s' {1..70})\",\"password\":\"y\"}")
    check "Create too-long username → 400" 400 "$S" || true

    S=$(DELETE "$JWT" "/api/users/alice")
    check "Delete user alice" 200 "$S" || true

    S=$(DELETE "$JWT" "/api/users/alice")
    check "Delete missing user → 404" 404 "$S" || true

    S=$(DELETE "$JWT" "/api/users/$ADMIN_USER")
    check "Cannot delete self → 400" 400 "$S" || true

    # Change own password (correct old)
    S=$(POST "$JWT" "/api/users/password" "{\"old_password\":\"$ADMIN_PASS\",\"new_password\":\"newpass\"}")
    check "Change own password (correct old)" 200 "$S" || true

    # Password changes revoke the current JWT. Re-login before changing the
    # password back, then re-login once more so later suites use a fresh token.
    S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"newpass\"}")
    check "Re-login after password change" 200 "$S" || true
    if [ "$S" = "200" ]; then
        JWT=$(body | jval .token)
    fi

    S=$(POST "$JWT" "/api/users/password" "{\"old_password\":\"newpass\",\"new_password\":\"$ADMIN_PASS\"}")
    check "Change password back" 200 "$S" || true

    S=$(PPOST "/api/login" "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
    check "Re-login after restoring password" 200 "$S" || true
    if [ "$S" = "200" ]; then
        JWT=$(body | jval .token)
    fi

    # Change with wrong old
    S=$(POST "$JWT" "/api/users/password" "{\"old_password\":\"wrong\",\"new_password\":\"x\"}")
    check "Change password wrong old → 401" 401 "$S" || true
fi
