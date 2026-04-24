# Emergency Rollback

If a CI auto-deploy breaks production (era service down, tasks failing unexpectedly):

```
ssh era@178.105.44.3
cd /opt/era
git log --oneline -10                # find last-good SHA
git reset --hard <SHA>
export PATH=/usr/local/go/bin:$PATH
make build
make docker-runner
sudo systemctl restart era
sudo systemctl status era            # confirm active (running)
```

Then fix the bad commit on master (revert or forward-fix) and push normally — CI will redeploy.
