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
