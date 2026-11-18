> [!TABS]
> #### Use a cache
> [!PROS]
> - One-line change, no schema change
> - `cache/` is not needed to run the app
> [!CONS]
> - Stale reads after a write
>
> #### Add an index
> [!PROS]
> - Keeps reads exact
> - Helps every query on the column
> [!CONS]
> - Slower writes
>
> #### Leave as is
> [!PROS]
> - Nothing to do
> [!CONS]
> - Slow page loads stay, for every user
