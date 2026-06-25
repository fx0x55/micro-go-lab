-- MySQL 8.4 多数据库初始化
-- MYSQL_DATABASE 环境变量会自动创建 users_db
-- 此脚本创建额外的 orders_db 数据库
CREATE DATABASE IF NOT EXISTS orders_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
