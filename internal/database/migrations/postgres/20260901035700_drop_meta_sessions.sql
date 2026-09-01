-- +goose Up
DROP TABLE IF EXISTS "sessions";
DROP TABLE IF EXISTS "platform_sessions";
DROP TABLE IF EXISTS "meta";

-- +goose Down
CREATE TABLE "meta" (
  "key" character varying NOT NULL,
  "value" character varying NOT NULL,
  "updated_at" timestamptz NOT NULL,
  PRIMARY KEY ("key")
);

CREATE TABLE "sessions" (
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "user_id" uuid NOT NULL,
  PRIMARY KEY ("token_hash"),
  CONSTRAINT "sessions_users_sessions" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
CREATE INDEX "session_expires_at" ON "sessions" ("expires_at");
CREATE INDEX "session_user_id" ON "sessions" ("user_id");

CREATE TABLE "platform_sessions" (
  "token_hash" character varying NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "platform_user_id" uuid NOT NULL,
  PRIMARY KEY ("token_hash"),
  CONSTRAINT "platform_sessions_platform_users_sessions" FOREIGN KEY ("platform_user_id") REFERENCES "platform_users" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
CREATE INDEX "platformsession_platform_user_id" ON "platform_sessions" ("platform_user_id");
CREATE INDEX "platformsession_expires_at" ON "platform_sessions" ("expires_at");
