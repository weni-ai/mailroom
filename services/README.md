# Services

## Description
Mailroom services bring together a set of services, usually external, with which Mailroom maintains some interaction, sending or consuming data via APIs of each service.

### Practical Example
An example of Mailroom interacting with an external service would be the integration with a ticket management system. Mailroom can:
1. Send a request to create a new ticket in an external system, including data such as problem description, priority and customer information.
2. Receive updates on this ticket, such as status changes or comments added by technical support.
3. Synchronize data between the external system and Mailroom's internal data, ensuring consistency and visibility for users.

## Service structure
- Currently, Mailroom has the following types of services: **external**, **ivr** and **tickets**.

### Service Type Table
| Type | Description |
|-----------|--------------------------------------------------------|
| External | Any external service that does not fall into the other categories. |
| IVR | External services that provide IVR solutions. |
| Tickets | External ticket management services. |

- Each service is divided into **service**, **client**, **utils** and **web**, with their respective unit test files.
- `service.go`: Responsible for the business rules that each service imposes, implementing the functions used by the web and client.
- `client.go`: Communicates with the external service through APIs, consuming and sending data.
- `web.go`: Interface that the external service uses to communicate with Mailroom. This file implements the endpoints necessary for communication.
- `utils.go`: Auxiliary file with service-specific utility functions (present in some services).

#### Unit Test Organization
Unit tests are organized by package and use widely known frameworks, such as `testify`. Tests typically simulate API calls using `MockRequestor` or similar implementations, ensuring that behavior is validated without real external dependencies.

Each type of service has its own test suite located within the corresponding directory.

## Service base structure
```go
// Example of base structure
type service struct {
rtConfig *runtime.Config // Settings
restClient *Client // HTTP Client for external calls
}
```

- Each service has its own specifications, which can be added to the struct in addition to the default ones.
- The `service.go` file also contains the constructor methods for each service: `NewService()`.
- Each service has its own specific methods, for example the method for opening tickets: `Open()`.

## Client base structure
```go
// Example of base structure
type baseClient struct {
httpClient *http.Client // HTTP client used for calls
httpRetries *httpx.RetryConfig // Settings for retries for failed requests
}
```

- Just like services, the client also has a constructor and its respective methods.
- By default, the client implements the methods:
- `get()`
- `post()`
- `put()`
- `delete()`
- `request()`