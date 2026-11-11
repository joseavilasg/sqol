-- A list of rows, and the conflict clause that keeps a repeat harmless.
INSERT INTO library.tags (name, kind)
VALUES
	('classic', 'genre'),
	('science', 'genre'),
	('signed', 'copy')
ON CONFLICT (name) DO NOTHING;

-- Three assignments in one SET: the first keeps the keyword's line and the
-- other two hang one level below it.
UPDATE library.members
SET username = $1,
	password_hash = $2,
	verified = TRUE
WHERE id = $3;
