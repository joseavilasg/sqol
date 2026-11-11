-- Shelf occupancy: the CTE that counts overdue loans, the one that knows the
-- new shelves, and a union with the library total.
WITH overdue AS (
	SELECT
		b.shelf_id,
		COUNT(*) AS total
	FROM library.loans AS l
		JOIN library.books AS b ON b.id = l.book_id
	WHERE l.returned_at IS NULL
		AND l.due_at < (NOW() AT TIME ZONE 'utc')
	GROUP BY b.shelf_id
),
recent AS (
	SELECT shelf_id
	FROM library.shelves
	WHERE created_at > (NOW() AT TIME ZONE 'utc') - INTERVAL '1 day'
)
SELECT
	s.name,
	COALESCE(o.total, 0) AS overdue,
	CASE WHEN o.total > 10 THEN 'busy' ELSE 'quiet' END AS state,
	RANK() OVER (ORDER BY COALESCE(o.total, 0) DESC) AS position
FROM library.shelves AS s
	LEFT JOIN overdue AS o ON o.shelf_id = s.id
	LEFT JOIN recent AS r ON r.shelf_id = s.id
GROUP BY s.name, o.total
HAVING COALESCE(o.total, 0) >= 0

UNION ALL

SELECT 'all', COUNT(*), 'total', 0
FROM library.books;
