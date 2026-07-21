#!/usr/bin/env bash
# 死锁 lab 的确定性复现器：开两个并发 MySQL 会话，对同一批 products 行做反序加锁（AB-BA），
# 100% 触发 InnoDB 死锁（ER_LOCK_DEADLOCK, 1213）。真实生产没有 sleep，但反序加锁高并发下照样爆。
#
# 本脚本只负责"造事"：触发死锁 + 让 InnoDB 回滚 victim。
# 抓现场（死锁图）留给读者练手，见 docs/troubleshooting/database-deadlock.md「调试」一节：
#   docker compose exec mysql mysql -uroot -proot inventory_db -e "SHOW ENGINE INNODB STATUS\G"
#
# 前置：make infra（或 docker compose -f docker-dev.yml up -d mysql）已起 MySQL。
set -euo pipefail

DBHOST="${DATABASE_HOST:-127.0.0.1}"
DBPORT="${DATABASE_PORT:-3306}"
DBUSER="${DATABASE_USER:-root}"
DBPASS="${DATABASE_PASSWORD:-root}"
MYSQL="mysql -h${DBHOST} -P${DBPORT} -u${DBUSER} -p${DBPASS} inventory_db"

echo "==> 1) 确认 inventory_db.products 有 SKU-001/SKU-002（没有则插入种子数据）"
$MYSQL -N -e "INSERT IGNORE INTO products (sku,total,available) VALUES ('SKU-001',100,100),('SKU-002',50,50);"
$MYSQL -e "SELECT sku,total,available FROM products WHERE sku IN ('SKU-001','SKU-002');"

SLEEP=${BUG_DEADLOCK_SLEEP_MS:-300}
SECS=$(awk "BEGIN{printf \"%.3f\", ${SLEEP}/1000}")

# 会话 A：先锁 SKU-001，sleep，再锁 SKU-002。
sessionA() {
  $MYSQL <<SQL
SET autocommit=0;
UPDATE products SET available = available - 0 WHERE sku = 'SKU-001';
DO SLEEP(${SECS});
UPDATE products SET available = available - 0 WHERE sku = 'SKU-002';
COMMIT;
SQL
}

# 会话 B：先锁 SKU-002，sleep，再锁 SKU-001 —— 与 A 反序。
sessionB() {
  $MYSQL <<SQL
SET autocommit=0;
UPDATE products SET available = available - 0 WHERE sku = 'SKU-002';
DO SLEEP(${SECS});
UPDATE products SET available = available - 0 WHERE sku = 'SKU-001';
COMMIT;
SQL
}

echo "==> 2) 并发跑两个反序加锁会话（sleep=${SECS}s）。预期其中一个会收到 1213 Deadlock。"
# A 先起、抢到 SKU-001 后 sleep；B 抢 SKU-002 后要 SKU-001（等 A），A 醒来要 SKU-002（等 B）→ 环。
sessionA &
PID_A=$!
sleep "$(awk "BEGIN{printf \"%.3f\", ${SECS}/2}")"
sessionB &
PID_B=$!

# 不论哪方被回滚，都算复现成功；打印各自退出码。
A_CODE=0; B_CODE=0
wait "$PID_A" || A_CODE=$?
wait "$PID_B" || B_CODE=$?

echo
echo "==> session A 退出码=${A_CODE} / session B 退出码=${B_CODE}"
echo "==> 若任一会话报 'ERROR 1213 (40001): Deadlock found' 即复现成功。"
echo "==> 现在去抓死锁图："
echo "    docker compose exec mysql mysql -uroot -proot inventory_db -e 'SHOW ENGINE INNODB STATUS\\G' | grep -A40 'LATEST DETECTED DEADLOCK'"
echo "==> 或在 Grafana/Loki 搜：{service=\"mysql\"} |~ \"DEADLOCK|TRANSACTION\""
