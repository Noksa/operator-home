# Migration Guide

## Which Kubernetes client is used in operator mode?

When you use the `Bootstrapper` to run a standard operator, the Kubernetes client comes from
**controller-runtime** — not from `operatorkclient`.

`NewBootstrapper` calls `ctrl.GetConfigOrDie()` internally to create the `manager.Manager`.
The manager owns its own `rest.Config`, typed client, and cache. Your reconcilers receive
objects through the manager's cached client, and you interact with the cluster via
`mgr.GetClient()`.

`operatorkclient.Client` is a **separate, lower-level client** designed for operations that
controller-runtime's client doesn't cover well:

- Executing commands inside pods (`RunCommandInPod`)
- Copying files to/from containers (`CopyFileToContainerInPod`)
- Reading raw pod logs (`GetPodContainerLogs`)
- Unstructured/dynamic access to arbitrary resources (`Dynamic()`)
- REST mapper and discovery for runtime GVR resolution

In a typical operator, you'd create an `operatorkclient.Client` (or use `DefaultClient()`)
only when your reconciler needs one of these capabilities. For standard CRUD on Kubernetes
objects, use the manager's client.

## Breaking changes

### `operatorkclient` — globals removed, struct-based API

**Old (package-level functions):**
```go
operatorkclient.SetClientSet(fake.NewClientset())
operatorkclient.SetConfigProvider(myProvider)
operatorkclient.InitializeOperatorCoreClientSet()

cfg := operatorkclient.GetClientConfig()
cs  := operatorkclient.GetClientSet()

logs, err := operatorkclient.GetPodContainerLogs(ctx, ns, pod, container, since)
stdout, stderr, err := operatorkclient.RunCommandInPod(cmd, container, pod, ns, nil)
```

**New (instance methods on `*Client`):**
```go
// Option A: create explicitly
client, err := operatorkclient.NewClient()                       // from kubeconfig / in-cluster
client, err := operatorkclient.NewClientFromConfig(cfg)          // from a specific rest.Config

// Option B: lazy singleton (convenient for operators that don't need multiple clients)
client, err := operatorkclient.DefaultClient()

// All operations are methods
cfg  := client.Config()
cs   := client.ClientSet()
dyn  := client.Dynamic()
disc := client.Discovery()
rm   := client.RESTMapper()

logs, err := client.GetPodContainerLogs(ctx, ns, pod, container, since)
stdout, stderr, err := client.RunCommandInPod(cmd, container, pod, ns, nil)
```

**For tests:**
```go
// Old
operatorkclient.SetClientSet(fake.NewClientset())
operatorkclient.SetConfigForTesting(cfg)
defer operatorkclient.ResetForTesting()

// New — no globals to reset
client := operatorkclient.NewClientFromClientSet(
    fake.NewClientset(),
    fakedynamic.NewSimpleDynamicClient(scheme.Scheme),
    cfg, // can be nil if you don't test SPDY operations
)
```

### Removed functions

| Old function                          | Replacement                                  |
|---------------------------------------|----------------------------------------------|
| `SetClientSet(cs)`                    | `NewClientFromClientSet(cs, dyn, cfg)`       |
| `GetClientSet()`                      | `client.ClientSet()`                         |
| `SetConfigProvider(fn)`               | `NewClientFromConfig(fn())`                  |
| `GetClientConfig()`                   | `client.Config()`                            |
| `InitializeOperatorCoreClientSet()`   | Removed — initialization is in constructors  |
| `ResetForTesting()`                   | Not needed — create a new `*Client` per test |
| `SetConfigForTesting(cfg)`            | Not needed — pass `cfg` to constructor       |


### New: dynamic client, discovery, REST mapper

`*Client` now includes `dynamic.Interface`, `discovery.CachedDiscoveryInterface`, and
`*restmapper.DeferredDiscoveryRESTMapper` out of the box. Access them via:

```go
client.Dynamic()     // dynamic.Interface — unstructured CRUD, server-side apply
client.Discovery()   // discovery.CachedDiscoveryInterface — API group/version queries
client.RESTMapper()  // *restmapper.DeferredDiscoveryRESTMapper — GVK ↔ GVR mapping
```

### `operatorbootstrap` — signal handling and cancellation

**Old (package-level function + global state):**
```go
ctx := operatorbootstrap.CustomSignalsHandler(cleanup)
if operatorbootstrap.Cancelled() { ... }
```

**New (methods on `*Bootstrapper`):**
```go
b := operatorbootstrap.NewBootstrapper(ctx, cfg, optsFunc, mgrFunc)
ctx = b.SetupSignalHandler(cleanup)
if b.Cancelled() { ... }
```

The `cancelled` flag is now an `atomic.Bool` inside the `Bootstrapper` — no more data race.

### `operatorbootstrap.AddPodIndexersToManager`

**Old (mutable global variable):**
```go
// Could be reassigned
operatorbootstrap.AddPodIndexersToManager = myCustomFunc
```

**New (plain function):**
```go
// Immutable — pass it directly or wrap it
operatorbootstrap.NewBootstrapper(ctx, cfg, optsFunc, operatorbootstrap.AddPodIndexersToManager)
```

If you need custom indexer logic, write your own `ManagerFunc` and pass it instead.

### `operatorconfig.CustomLoggerSetup`

**Old (package-level mutable variable):**
```go
operatorconfig.CustomLoggerSetup = func() logr.Logger { return myLogger }
```

**New (field on `DefaultConfig`):**
```go
type MyConfig struct {
    operatorconfig.DefaultConfig
}

func (c *MyConfig) Initialize() error {
    c.CustomLoggerSetup = func() logr.Logger { return myLogger }
    return nil
}
```

Or set it before passing the config to `NewBootstrapper`:
```go
cfg := &MyConfig{}
cfg.CustomLoggerSetup = func() logr.Logger { return myLogger }
b := operatorbootstrap.NewBootstrapper(ctx, cfg, optsFunc, mgrFunc)
```

## Quick migration checklist

1. Replace all `operatorkclient.<Function>(...)` calls with `client.<Method>(...)` on a `*Client` instance
2. Choose your client creation strategy: `NewClient()`, `NewClientFromConfig(cfg)`, or `DefaultClient()`
3. In tests, use `NewClientFromClientSet(cs, dyn, cfg)` — no more `ResetForTesting`
4. Replace `CustomSignalsHandler(fn)` with `bootstrapper.SetupSignalHandler(fn)`
5. Replace `operatorbootstrap.Cancelled()` with `bootstrapper.Cancelled()`
6. Move `operatorconfig.CustomLoggerSetup` into your config struct's `CustomLoggerSetup` field
7. If you were reassigning `AddPodIndexersToManager`, write a custom `ManagerFunc` instead
