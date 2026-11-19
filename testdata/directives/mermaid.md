# Diagrams

A flowchart:

```mermaid
flowchart TD
  A[Start] --> B{Ready?}
  B -->|yes| C[Ship]
  B -->|no| D[Fix]
  D --> B
```

A sequence:

```mermaid
sequenceDiagram
  participant U as User
  participant S as Server
  U->>S: GET /items
  S-->>U: 200 OK
```

An entity relation:

```mermaid
erDiagram
  CUSTOMER ||--o{ ORDER : places
  ORDER ||--|{ LINE : contains
```

Too wide, shows as code:

```mermaid
graph LR
  Alpha --> Bravo --> Charlie --> Delta --> Echo --> Foxtrot --> Golf --> Hotel
```

A parse error, shows as code:

```mermaid
sequenceDiagram
  A->>
```

Not a type it draws, shows as code:

```mermaid
pie title Pets
  "Dogs" : 3
  "Cats" : 2
```

In a list it stays code:

- Item

  ```mermaid
  graph LR
    A --> B
  ```

The end.
