'use strict';

// Minimal forward-only migration runner: applies migrations/*.sql in filename
// order, each in a transaction, tracking applied files in a `_migrations` table.
//   node src/db/migrate.js        (uses DATABASE_URL / config.postgres.url)
require('dotenv').config();

const fs = require('fs');
const path = require('path');
const { Pool } = require('pg');

const config = require('../../config/default');

const MIGRATIONS_DIR = path.join(__dirname, '..', '..', 'migrations');

async function migrate(url) {
  const pool = new Pool({ connectionString: url });
  try {
    await pool.query(
      `CREATE TABLE IF NOT EXISTS _migrations (
         name TEXT PRIMARY KEY,
         applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
       )`
    );
    const applied = new Set(
      (await pool.query('SELECT name FROM _migrations')).rows.map((r) => r.name)
    );
    const files = fs
      .readdirSync(MIGRATIONS_DIR)
      .filter((f) => f.endsWith('.sql'))
      .sort();

    let ran = 0;
    for (const file of files) {
      if (applied.has(file)) continue;
      const sql = fs.readFileSync(path.join(MIGRATIONS_DIR, file), 'utf8');
      const client = await pool.connect();
      try {
        await client.query('BEGIN');
        await client.query(sql);
        await client.query('INSERT INTO _migrations (name) VALUES ($1)', [file]);
        await client.query('COMMIT');
        console.log(`[migrate] applied ${file}`);
        ran += 1;
      } catch (err) {
        await client.query('ROLLBACK');
        throw new Error(`migration ${file} failed: ${err.message}`);
      } finally {
        client.release();
      }
    }
    console.log(ran ? `[migrate] applied ${ran} migration(s)` : '[migrate] up to date');
  } finally {
    await pool.end();
  }
}

if (require.main === module) {
  const url = process.env.DATABASE_URL || (config.postgres && config.postgres.url);
  if (!url) {
    console.error('[migrate] DATABASE_URL (or config.postgres.url) is not set');
    process.exit(1);
  }
  migrate(url).catch((err) => {
    console.error(err.message);
    process.exit(1);
  });
}

module.exports = { migrate };
