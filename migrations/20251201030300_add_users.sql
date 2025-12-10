CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
  id         uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  name       varchar(150) NOT NULL,
  email      varchar(200) NOT NULL UNIQUE,
  password   varchar(255) NOT NULL, -- bcrypt hash
  role       varchar(50)  NOT NULL DEFAULT 'admin',
  created_at timestamptz  NOT NULL DEFAULT now(),
  updated_at timestamptz  NOT NULL DEFAULT now()
);
