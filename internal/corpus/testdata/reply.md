# Moving to Postgres

Short answer: **yes**, once writes are concurrent.

## 1. Why

> [!WARNING] Downtime
> The migration locks the table for a few seconds.

> [!TABS]
#### macOS
```sh
brew install postgresql
#### not a heading, inside code
```

#### Linux
> [!PROS]
- packaged everywhere

> [!CONS]
- needs a service

---

> [!STEPS] The plan
1. **Back up**
   *Day 1*
   Dump the database.
2. **Switch**
   *Day 2*
   Point the app at Postgres.

After the steps, a paragraph.

> [!STATS]
- **3×** faster writes
- **−40%** p99

| Option | Cost |
| ------ | ---- |
| SQLite | 0 |

- [ ] tell the team
- [x] write the plan

```tree
src/
└── db.ts  # the client
```