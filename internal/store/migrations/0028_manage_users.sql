-- Manage users (#159) replaces "has every Manager switch" for the Users page. Everyone who had every
-- Manager switch keeps the Users page: they get the new switch too.
UPDATE users SET perms = perms || ',manageUsers'
WHERE instr(',' || perms || ',', ',manageRequests,') > 0 AND instr(',' || perms || ',', ',autoApprove,') > 0
  AND instr(',' || perms || ',', ',requestMovies,') > 0 AND instr(',' || perms || ',', ',requestTv,') > 0
  AND instr(',' || perms || ',', ',advanced,') > 0 AND instr(',' || perms || ',', ',reportIssues,') > 0
  AND instr(',' || perms || ',', ',manageIssues,') > 0 AND instr(',' || perms || ',', ',redownload,') > 0
  AND instr(',' || perms || ',', ',manageUsers,') = 0;
