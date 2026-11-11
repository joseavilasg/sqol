-- Title of every loan that is still out, with the member holding it.
SELECT m.username, l.loan_id, b.title
FROM library.loans AS l
	JOIN library.books AS b ON b.id = l.book_id
	LEFT JOIN library.members AS m
		ON m.id = l.member_id
		AND m.verified
WHERE l.returned_at IS NULL
	AND (
		l.due_at < (NOW() AT TIME ZONE 'utc')
		OR l.renewed > 2
	)
ORDER BY l.due_at;
