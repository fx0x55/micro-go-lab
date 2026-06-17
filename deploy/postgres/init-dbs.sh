#!/bin/bash
# Create multiple databases on first startup.
# POSTGRES_DB (users_db) is created automatically by the postgres image;
# this script creates the additional databases listed in POSTGRES_EXTRA_DBS.

set -e

for db in $(echo "$POSTGRES_EXTRA_DBS" | tr ',' ' '); do
  echo "Creating database: $db"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-SQL
    CREATE DATABASE "$db";
SQL
done
