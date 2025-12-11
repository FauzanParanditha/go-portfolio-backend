-- Add FULL case-study fields to projects table
ALTER TABLE projects
    ADD COLUMN long_desc text,
    ADD COLUMN category varchar(150),
    ADD COLUMN timeline varchar(150),
    ADD COLUMN role varchar(150),
    ADD COLUMN challenge text,
    ADD COLUMN solution text,
    ADD COLUMN results text[] DEFAULT '{}',
    ADD COLUMN technical_details jsonb,
    ADD COLUMN demo_url text,
    ADD COLUMN repo_url text;

-- Optional: remove old fields live_url + source_url
ALTER TABLE projects
    DROP COLUMN IF EXISTS live_url,
    DROP COLUMN IF EXISTS source_url;

-- Screenshots table for project galleries
CREATE TABLE IF NOT EXISTS project_screenshots (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    image_url text NOT NULL,
    sort_order int NOT NULL DEFAULT 0
);
