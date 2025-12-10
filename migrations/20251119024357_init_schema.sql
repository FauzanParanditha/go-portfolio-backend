-- 20251119100000_init_schema.sql

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE tags (
  id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  name       varchar(100) NOT NULL,
  type       varchar(50),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE experiences (
  id          uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  title       varchar(150) NOT NULL,
  company     varchar(150) NOT NULL,
  location    varchar(150),
  start_date  date NOT NULL,
  end_date    date,
  is_current  boolean NOT NULL DEFAULT false,
  description text,
  sort_order  int NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE experience_highlights (
  id             uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  experience_id  uuid NOT NULL REFERENCES experiences(id) ON DELETE CASCADE,
  text           text NOT NULL,
  sort_order     int NOT NULL DEFAULT 0
);

CREATE TABLE projects (
  id              uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  title           varchar(150) NOT NULL,
  slug            varchar(150) NOT NULL UNIQUE,
  short_desc      text,
  cover_image_url text,
  live_url        text,
  source_url      text,
  is_featured     boolean NOT NULL DEFAULT false,
  sort_order      int NOT NULL DEFAULT 0,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE project_features (
  id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  text       text NOT NULL,
  sort_order int NOT NULL DEFAULT 0
);

CREATE TABLE experience_tags (
  experience_id uuid NOT NULL REFERENCES experiences(id) ON DELETE CASCADE,
  tag_id        uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (experience_id, tag_id)
);

CREATE TABLE project_tags (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  tag_id     uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (project_id, tag_id)
);

CREATE TABLE contact_messages (
  id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  name       varchar(150) NOT NULL,
  email      varchar(200) NOT NULL,
  subject    varchar(200),
  message    text NOT NULL,
  status     varchar(50) NOT NULL DEFAULT 'new',
  created_at timestamptz NOT NULL DEFAULT now()
);
