This directory holds the assertion that must NOT compile.

`run.sh` builds it in a throwaway module and FAILS the experiment if it succeeds.
The expected error is:

    impossible type assertion: v.(s.MapSet[string])
        sealing.MapSet[string] does not implement sealing.IfaceView[string]
        (missing method viewOnly)

`MapSet[string]` deliberately matches `IfaceView[string]` on every OTHER method,
so `viewOnly` is the only thing named. An earlier version used
`MapSet[*Item]`, whose `Keys()` has the wrong element type -- the assertion was
impossible for that reason instead, and the check would have kept passing even
with the seal removed.
