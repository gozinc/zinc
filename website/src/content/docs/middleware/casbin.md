---
title: Casbin Auth
description: Decide whether a signed-in user may make a request, using a Casbin policy.
---

Casbin Auth decides whether the current user may make a request, and answers `403` when the answer is no. You write the rules as a [Casbin](https://casbin.org) policy, such as "alice may `POST` to `/reports/*`", instead of in each handler. It runs after an auth middleware that has already worked out who the user is.

## Usage

The middleware works with any value that has an `Enforce(args ...any) (bool, error)` method, so Zinc doesn't depend on Casbin itself. The Casbin library's own enforcer fits:

```go
import (
	"github.com/0mjs/zinc/middleware/basicauth"
	"github.com/0mjs/zinc/middleware/casbin"
	casbinlib "github.com/casbin/casbin/v2"
)

enforcer, err := casbinlib.NewEnforcer("model.conf", "policy.csv")
if err != nil {
	log.Fatal(err)
}

reports := app.Group("/reports",
	// Who is it? checkUser is your basicauth.Validator.
	basicauth.New(basicauth.Config{Validator: checkUser}),
	// May they do this?
	casbin.New(casbin.Config{
		Enforcer: enforcer,
		Subject:  casbin.SubjectFromBasicAuth(),
	}),
)
```

With this model and policy:

```ini
# model.conf
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && keyMatch(r.obj, p.obj) && r.act == p.act
```

```text
# policy.csv
p, alice, /reports/*, GET
p, alice, /reports/*, POST
p, bob, /reports/*, GET
```

```bash
curl -u bob:b-pass http://localhost:8080/reports/7
# {"id":"7"}

curl -i -X POST -u bob:b-pass http://localhost:8080/reports/7
# HTTP/1.1 403 Forbidden
# {"error":{"status":403,"message":"Forbidden"}}

curl -i -X POST -u alice:a-pass http://localhost:8080/reports/7
# HTTP/1.1 204 No Content
```

Register it after the middleware that identifies the user, so the subject is known when the policy runs.

## Defaults

| Setting | Default |
|---|---|
| `Enforcer` | **required**: `New` panics without it |
| `Subject` | **required**: `New` panics without it |
| Object | the request path, `c.Path()` |
| Action | the request method, `c.Method()` |
| On allow | calls `c.Next()` |
| On deny | `403 Forbidden` |

Each request calls `enforcer.Enforce(subject, object, action)`, in that order.

## Configuration

Take the user from a value an earlier middleware stored, and rename the denial:

```go
app.Use(casbin.New(casbin.Config{
	Enforcer: enforcer,
	Subject:  casbin.SubjectFromContext(userKey{}), // set with c.Set(userKey{}, name)
	Object:   casbin.ObjectPath(),
	Action:   casbin.ActionMethod(),
	ErrorHandler: func(c *zinc.Context, err error) error {
		if errors.Is(err, casbin.ErrRejected) {
			return zinc.NewError(http.StatusForbidden, "you can't do that")
		}
		return err
	},
}))
```

When the policy has no rule for the user, here `carol`:

```bash
curl http://localhost:8080/reports/1
# {"error":{"status":403,"message":"you can't do that"}}
```

| Field | Default | Meaning |
|---|---|---|
| `Enforcer` | required | Anything with `Enforce(args ...any) (bool, error)`, such as a Casbin enforcer. |
| `Subject` | required | Returns who is asking: the first argument to `Enforce`. |
| `Object` | `casbin.ObjectPath()` | Returns what they're asking for: the second argument. |
| `Action` | `casbin.ActionMethod()` | Returns what they want to do: the third argument. |
| `SuccessHandler` | calls `c.Next()` | Runs when the policy allows the request. Call `c.Next()` in it to continue. |
| `ErrorHandler` | `403` on denial | Runs when the policy denies the request or `Enforce` fails. Its return value is what the client gets. |

`Subject`, `Object` and `Action` are all `casbin.ValueFunc`, a `func(*zinc.Context) any`, so you can return any value your model expects.

## Choose the subject

| Helper | Subject |
|---|---|
| `casbin.SubjectFromBasicAuth()` | The username from [Basic Auth](/middleware/basicauth/) |
| `casbin.SubjectFromKeyAuth()` | The key from [Key Auth](/middleware/keyauth/) |
| `casbin.SubjectFromContext(key)` | Whatever an earlier middleware stored with `c.Set(key, value)` |

If the auth middleware didn't run, these return an empty string (or `nil` for `SubjectFromContext`), which a typical policy denies.

`casbin.ObjectPath()` and `casbin.ActionMethod()` are the default object and action. Pass your own `ValueFunc` to use the route pattern (`c.FullPath()`) or a resource name instead.

## Errors

| Error | Client gets |
|---|---|
| `casbin.ErrRejected`: the policy denied the request | `403 Forbidden` |
| An error from `Enforce`, such as a broken policy | the app's error handler; `500` by default |

Both go through `ErrorHandler` first, so use `errors.Is(err, casbin.ErrRejected)` there to tell a denial from a failure.

## Related

- [Basic Auth](/middleware/basicauth/) and [Key Auth](/middleware/keyauth/): identify the user before the policy runs.
- [JWT](/middleware/jwtauth/): identify the user from a token, then pass a claim with `SubjectFromContext` or your own `ValueFunc`.
- [Groups and Middleware](/guide/groups-and-middleware/): apply the policy to one group of routes.
