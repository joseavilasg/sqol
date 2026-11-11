-- A body with every block the formatter has to follow: a declaration list, a
-- loop, an IF with an ELSIF, and an ELSE that all return to their own level.
DO $$
DECLARE
	total BIGINT;
	shelf record;
BEGIN
	FOR shelf IN SELECT id FROM library.shelves LOOP
		SELECT COUNT(*) INTO total
		FROM library.books
		WHERE shelf_id = shelf.id;
		IF total = 0 THEN
			RAISE NOTICE 'empty shelf';
		ELSIF total > 100 THEN
			RAISE NOTICE 'full shelf';
		ELSE
			RAISE NOTICE 'shelf holds %', total;
		END IF;
	END LOOP;
END $$;
