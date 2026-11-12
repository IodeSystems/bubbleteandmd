# Diff fences

A change, no title (plain code in the tint):

```diff
@@ -1,4 +1,4 @@ export default {
   entry: "src/index.ts",
-  typecheck: true,
-  minify: false,
+  typecheck: false,
+  minify: true,
 }
```

Titled, Go colours inside the tint:

```diff title="internal/app/run.go"
diff --git a/internal/app/run.go b/internal/app/run.go
index 3b18e51..a9c4f0d 100644
--- a/internal/app/run.go
+++ b/internal/app/run.go
@@ -12,7 +12,8 @@ func Run(ctx context.Context) error {
 	cfg, err := load(ctx)
 	if err != nil {
-		return nil
+		return fmt.Errorf("load: %w", err)
 	}
+	log.Printf("config %s loaded from a path that makes this line wider than the pane", cfg.Name)
 	return serve(ctx, cfg)
 }
\ No newline at end of file
```

```patch
-removed only
+added only, nothing shared with it
-unpaired removal
```

> [!NOTE]
> In a callout:
>
> ```diff
> -old()
> +new()
> ```

```diff title="notes.unknownext"
-plain old text
+plain new text
```

```diff
```

The end.
