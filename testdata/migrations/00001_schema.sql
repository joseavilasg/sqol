-- A table with the four shapes the column rule has to line up: text, number,
-- boolean and a timestamp, plus a primary key that is not a column.
CREATE TABLE library.members (
	id            TEXT      NOT NULL,
	member_no     BIGINT    NOT NULL,
	username      TEXT      NOT NULL,
	password_hash TEXT      NOT NULL,
	default_shelf TEXT      NOT NULL DEFAULT '',
	total_pages   BIGINT             DEFAULT 0,
	max_pages     BIGINT             DEFAULT 1000000000000,
	is_active     BOOLEAN            DEFAULT TRUE,
	verified      BOOLEAN            DEFAULT FALSE,
	created_at    TIMESTAMP NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
	CONSTRAINT members_pkey PRIMARY KEY (id)
);

CREATE UNIQUE INDEX members_identity_shelf_key
	ON library.members (member_no, default_shelf);

CREATE INDEX books_shelf_idx
	ON library.books (shelf_id)
WHERE shelf_id IS NOT NULL;
