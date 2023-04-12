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
