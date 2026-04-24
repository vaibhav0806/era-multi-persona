# Rotate the CI Deploy Key

If `DEPLOY_SSH_KEY` leaks or needs periodic rotation:

1. Generate new keypair on Mac:
   ```
   ssh-keygen -t ed25519 -f ~/.ssh/era_ci_new -C "era-ci-$(date +%Y%m)" -N ""
   ```

2. Append new public key to VPS:
   ```
   cat ~/.ssh/era_ci_new.pub | ssh era@178.105.44.3 'cat >> ~/.ssh/authorized_keys'
   ```

3. Update GitHub secret:
   ```
   gh secret set DEPLOY_SSH_KEY --repo vaibhav0806/era < ~/.ssh/era_ci_new
   ```

4. Trigger a CI deploy (empty commit, or workflow_dispatch) to confirm new key works.

5. Remove the old public key from VPS authorized_keys:
   ```
   ssh era@178.105.44.3
   vi ~/.ssh/authorized_keys        # delete the old "era-ci@github-actions" line
   ```

6. Delete old local keypair:
   ```
   shred -u ~/.ssh/era_ci ~/.ssh/era_ci.pub
   mv ~/.ssh/era_ci_new ~/.ssh/era_ci
   mv ~/.ssh/era_ci_new.pub ~/.ssh/era_ci.pub
   ```
