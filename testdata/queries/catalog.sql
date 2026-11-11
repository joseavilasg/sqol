-- Returns one page of a shelf, newest first.
SELECT
	b.id,
	-- the client shows the title and nothing else
	b.title,
	'AND OR WHERE' AS note,
	COALESCE(b.pages, 0) AS pages,
	(
		SELECT COUNT(*)
		FROM library.loans AS l
		WHERE l.book_id = b.id
	) AS loans
FROM library.books AS b
WHERE b.shelf_id IS NOT DISTINCT FROM $1
	AND b.state = 'available'
ORDER BY b.created_at DESC
LIMIT $2
OFFSET $3;
