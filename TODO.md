# dbx-dash — TODO

## Verification

- [ ] Verify that `CAN_USE` and `CAN_MANAGE` permission levels for service principals are
      correctly fetched and displayed:
      - In the **SP detail popup** → "WHO HAS ACCESS" section (shows users/groups with
        workspace permissions on the SP, e.g. `CAN_USE`, `CAN_MANAGE`, `IS_OWNER`)
      - In the **User detail popup** → "SERVICE PRINCIPAL ACCESS" section (shows which SPs
        the user can reach via group co-membership, with the via-groups chain)
      Test by: picking an SP with known ACL → pressing Enter on it → verifying the section
      shows correct principals and levels.
