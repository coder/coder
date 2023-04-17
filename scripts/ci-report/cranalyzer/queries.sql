-- Show top failing tests and packages on main.
select * from (
    select
        t.package, t.name, j.name, count(jr.*) as fails, count(case when jr.timeout then true else null end) AS timeouts
    from job_results jr
    join tests t on (jr.test_id = t.id)
    join jobs j on (jr.job_id = j.id)
    join runs r on (j.run_id = r.id)
    where
        jr.status = 'fail'
        AND r.branch = 'main'
    group by t.package, t.name, j.name
) q1 order by fails desc;

-- Show failure stats for a test.
with t1 as (
	select
		r.branch,
		j.name AS platform,
		date_trunc('day', j.ts) AS d,
		output
	from job_results jr
	join jobs j on jr.job_id = j.id
	join runs r on j.run_id = r.id
	join tests t on jr.test_id = t.id
	where
		t.name = 'TestWorkspaceAgent_Metadata'
		and jr.status = 'fail'
		and jr.output not ilike '%duplicate migration%'
)

select branch, platform, d, count(*) as fails, array_agg(output) AS outputs
from t1
group by branch, platform, d
order by d, branch, platform;

-- Show tests that have failed on 'main' and later affected other branches (including main).
with t1 as (
	select
		r.branch,
		t.package,
		t.id as tid,
		t.name as name,
		j.name as platform,
		t.added,
		j.ts,
		date_trunc('day', j.ts) as d,
		output
	from job_results jr
	join jobs j on jr.job_id = j.id
	join runs r on j.run_id = r.id
	join tests t on jr.test_id = t.id
	where
		jr.status = 'fail'
		and jr.output not ilike '%duplicate migration%'
), main as (
	select distinct on (package, name) package, name, ts
	from t1
	where
		name is not null
		and branch = 'main'
	order by package, name, ts asc
), authors as (
	select t.id as tid, r.author_login, r.commit, r.commit_message
	from tests t
	join runs r on t.added = r.ts
)

select
	package,
	name,
	array_agg(platform) as platforms,
	array_agg(distinct d) as days,
	count(*) as fails,
	array_agg(distinct branch) as affected_branches,
	count(distinct branch) AS num_affected_branches,
	a.author_login,
	a.commit_message,
	min(added) as added,
	max(t1.ts) as last_fail,
	array_agg(output) as outputs
from t1
join main using (package, name)
join authors a on t1.tid = a.tid
where t1.ts > main.ts
group by t1.package, t1.name, a.author_login, a.commit_message
order by t1.package, t1.name;
