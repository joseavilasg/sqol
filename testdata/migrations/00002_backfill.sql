-- Recomputes the pages a shelf holds from the books that sit on it. The guard
-- keeps the migration usable on a schema that predates the column.
DO $$
BEGIN
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'library' AND table_name = 'shelves'
			AND column_name = 'total_pages'
	) THEN
		UPDATE library.shelves AS s
		SET total_pages = COALESCE(
			(
				SELECT SUM(b.pages)
				FROM library.books AS b
				WHERE b.shelf_id = s.id
			),
			0
		);
	ELSE
		RAISE NOTICE 'nothing to do';
	END IF;
END $$;
