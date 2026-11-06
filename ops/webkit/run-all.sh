#!/usr/bin/env bash
# Run every WebKit/iOS regression suite against a live ICS instance.
#   ./run-all.sh                                   # local test instance
#   ICS_TEST_URL=https://flow.bohea.us \
#   ICS_TEST_PASSWORD=<your password> ICS_TEST_KEY=<your key> ./run-all.sh
set -u
cd "$(dirname "$0")"
fail=0
for f in [0-9][0-9]-*.py; do
    echo
    echo "########## $f ##########"
    if python3 "$f"; then :; else fail=1; fi
done
echo
if [ "$fail" -eq 0 ]; then echo "===== ALL SUITES PASS ====="; else echo "===== SOME SUITES FAILED ====="; fi
exit "$fail"
