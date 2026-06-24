# cofiswarm-backend-llama

Cofiswarm component: `backend-llama`.

- Layout: [REPO-STANDARD-LAYOUT](https://github.com/keepdevops/cofiswarm-docs/blob/main/REPO-STANDARD-LAYOUT.md)
- Migration: [MIGRATION-SPRINTS](https://github.com/keepdevops/cofiswarm-docs/blob/main/MIGRATION-SPRINTS.md)

## FHS paths

| Path | Purpose |
|------|---------|
| `/etc/cofiswarm/backend-llama/` | config |
| `/var/lib/cofiswarm/backend-llama/` | state |
| `/var/log/cofiswarm/backend-llama/` | logs |

## Test

```bash
./test/scripts/assert-layout.sh backend-llama
```
