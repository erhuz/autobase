package migrations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"postgresql-cluster-console/internal/configuration"
)

const stock290Migration = 20260402170333

type stockConfigFixture struct {
	SourceVersion string            `json:"source_version"`
	Environment   map[string]string `json:"environment"`
}

func TestStock290Upgrade(t *testing.T) {
	dsn := os.Getenv("PG_CONSOLE_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_CONSOLE_TEST_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var publicTables int
	if err = pool.QueryRow(ctx, "select count(*) from pg_tables where schemaname = 'public'").Scan(&publicTables); err != nil {
		t.Fatal(err)
	}
	if publicTables != 0 {
		t.Fatal("stock upgrade test requires an empty database")
	}

	migrationDir, err := filepath.Abs("../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpToContext(ctx, db, migrationDir, stock290Migration); err != nil {
		t.Fatal(err)
	}

	fixtureSQL, err := os.ReadFile("testdata/2.9.0/fixture.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(fixtureSQL)); err != nil {
		t.Fatal(err)
	}

	configData, err := os.ReadFile("testdata/2.9.0/config.json")
	if err != nil {
		t.Fatal(err)
	}
	var configFixture stockConfigFixture
	if err = json.Unmarshal(configData, &configFixture); err != nil {
		t.Fatal(err)
	}
	if configFixture.SourceVersion != "2.9.0" {
		t.Fatalf("source version = %q", configFixture.SourceVersion)
	}
	for key, value := range configFixture.Environment {
		t.Setenv(key, value)
	}
	cfg, err := configuration.ReadConfig()
	if err != nil {
		t.Fatal(err)
	}

	if err = Migrate(pool, migrationDir); err != nil {
		t.Fatal(err)
	}

	t.Run("preserves stock data and external config", func(t *testing.T) {
		var preserved bool
		if err := pool.QueryRow(ctx, `
			select
				(select count(*) = 1 from public.projects where project_name = 'migration-fixture')
				and (select count(*) = 1 from public.environments where environment_name = 'migration-fixture')
				and (select count(*) = 1 from public.settings
					where setting_name = 'migration-fixture'
					  and setting_value = '{"enabled":true,"owner":"fixture-operator"}')
				and (select count(*) = 1 from public.clusters
					where cluster_name = 'migration-fixture'
					  and cluster_status = 'healthy'
					  and cluster_location = 'fixture-region'
					  and connection_info = '{"host":"db.example.test","port":5432}'
					  and extra_vars = '{"patroni_cluster_name":"migration-fixture","postgresql_exists":true}'
					  and inventory = '{"all":{"hosts":{"postgresql-1":{"ansible_host":"192.0.2.10"}}}}'
					  and server_count = 1
					  and postgres_version = 16
					  and flags = 9)
				and (select count(*) = 1 from public.servers
					where server_name = 'postgresql-1'
					  and ip_address = '192.0.2.10'
					  and timeline = 7)
				and (select get_secret(secret_id, $1)->>'username' = 'fixture-operator'
					from public.secrets where secret_name = 'migration-fixture')`,
			cfg.EncryptionKey).Scan(&preserved); err != nil {
			t.Fatal(err)
		}
		if !preserved {
			t.Fatal("stock project, environment, settings, server, or encrypted secret changed")
		}
		if cfg.Authorization.Token != "fixture-only-auth-token" ||
			cfg.Db.Host != "stock-console-db" ||
			cfg.Docker.Image != "autobase/automation:2.8.0" {
			t.Fatalf("stock config mismatch: host=%q image=%q", cfg.Db.Host, cfg.Docker.Image)
		}
	})

	t.Run("migrates operation history without launching work", func(t *testing.T) {
		var count int
		var status, operationLog, actor string
		if err := pool.QueryRow(ctx, `
			select count(*), min(operation_status), min(operation_log), min(actor)
			from public.operations
			where cluster_id = (select cluster_id from public.clusters where cluster_name = 'migration-fixture')`).
			Scan(&count, &status, &operationLog, &actor); err != nil {
			t.Fatal(err)
		}
		if count != 1 || status != "succeeded" || operationLog != "fixture operation completed" || actor != "api-token" {
			t.Fatalf("operation count=%d status=%q log=%q actor=%q", count, status, operationLog, actor)
		}
	})

	t.Run("adds analytics schema without opting in existing cluster", func(t *testing.T) {
		for _, relation := range []string{
			"public.query_analytics_sources",
			"public.query_analytics_fingerprints",
			"public.query_analytics_buckets",
			"public.query_analytics_samples",
			"public.query_analytics_credentials",
			"public.query_analytics_buckets_cluster_time_idx",
		} {
			var exists bool
			if err := pool.QueryRow(ctx, "select to_regclass($1) is not null", relation).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Errorf("%s missing", relation)
			}
		}

		var managed, desired bool
		var analyticsRows int
		if err := pool.QueryRow(ctx, `
			select query_analytics_managed, query_analytics_desired,
				(select count(*) from public.query_analytics_sources)
				+ (select count(*) from public.query_analytics_credentials)
			from public.clusters where cluster_name = 'migration-fixture'`).
			Scan(&managed, &desired, &analyticsRows); err != nil {
			t.Fatal(err)
		}
		if managed || desired || analyticsRows != 0 {
			t.Fatalf("existing cluster analytics managed=%t desired=%t rows=%d", managed, desired, analyticsRows)
		}
	})

	t.Run("adds empty backup evidence store", func(t *testing.T) {
		var exists bool
		var rows int
		if err := pool.QueryRow(ctx, "select to_regclass('public.cluster_backup_evidence') is not null").Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "select count(*) from public.cluster_backup_evidence").Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if !exists || rows != 0 {
			t.Fatalf("backup evidence relation exists=%t rows=%d", exists, rows)
		}
	})

	t.Run("reaches current migration", func(t *testing.T) {
		version, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if version != 20260723120000 {
			t.Fatalf("migration version = %d", version)
		}
	})
}
