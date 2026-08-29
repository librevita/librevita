-- +goose Up
-- Amendment chain: a new Episode may replace a finalized predecessor.
-- predecessor_id is unique so the chain stays linear (one successor).

ALTER TABLE "episodes" ADD COLUMN "predecessor_id" uuid NULL;
ALTER TABLE "episodes" ADD CONSTRAINT "episodes_episodes_amendment" FOREIGN KEY ("predecessor_id") REFERENCES "episodes" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
CREATE UNIQUE INDEX "episode_predecessor_id" ON "episodes" ("predecessor_id");

-- +goose Down
DROP INDEX IF EXISTS "episode_predecessor_id";
ALTER TABLE "episodes" DROP CONSTRAINT IF EXISTS "episodes_episodes_amendment";
ALTER TABLE "episodes" DROP COLUMN "predecessor_id";
