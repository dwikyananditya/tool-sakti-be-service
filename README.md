# Tool Sakti BE Service

Generate workbook KPI dari DSM pakai GitHub GraphQL API. Tiap user masuk dengan GitHub token sendiri.

## Jalankan

```bash
cd web && bun install && bun run build && cd ..
go run .
```

Buka `http://localhost:8080`, tempel token, upload DSM, unduh KPI.

## Ambil token

```bash
gh auth refresh -s repo -s read:org -s read:project
gh auth token
```
