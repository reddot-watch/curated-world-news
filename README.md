# Curated World News

This project aims to maintain a curated list of global news RSS feeds, initially focusing on English-language sources that have a high likelihood of reporting on security-related events such as terrorism, conflict, protests, crime, and consequences of environmental disasters (e.g., flooding, earthquakes). The majority of included sources naturally report global news across a multitude of topics. However, feeds explicitly targeting specialized categories such as, for example but not limited to, business, finance, technology, culture, tourism, or sports have been intentionally excluded.

## Source of Feeds

The list was created entirely from scratch by prompting several advanced language models, specifically:

- ChatGPT 4.5
- Claude Sonnet 3.5
- DeepSeek R1
- Gemini Advanced 2.0 (with search capabilities)

The only manual additions to the list so far are specialized Google News RSS search queries, for example:

```csv
https://news.google.com/rss/search?q=%22an+explosive%22+OR+%22suicide+attack%22+OR+%22suicide%22+OR+%22explosive+belt%22+OR+%22bomb+kills%22+OR+%22suicide+terrorist%22+OR+%22suicide+bomber%22+OR+%22explosive+device%22+OR+%22detonatin%22+OR+%22bomber+kills%22+OR+%22suicide+bombers%22+OR+%22bomb+exploded%22&hl=en-US&gl=US&ceid=US:en,global,en,active
```

## Project Structure

- `feeds.csv`: Curated list of RSS feed URLs along with their comments (geographical focus), language, and status.
- `validate_feeds.go`: Go script for concurrent validation of RSS feeds.
- `.github/workflows/validate-feeds.yml`: GitHub Actions workflow for periodic automated validation of RSS feed availability.

### Example from `feeds.csv`

```csv
url,comments,language,status
https://www.thehindu.com/news/national/kerala/feeder/default.rss,"Kerala, India",en,active
https://www.dailymailgh.com/feed/,Ghana,en,active
https://www.nhpr.org/rss.xml,New Hampshire,en,active
https://www.suchtv.pk/world.html?format=feed&type=rss,Pakistan,en,active
```

## Validation

We encourage collaboration to refine this list by adding or removing sources with a high likelihood of reporting on security-related events, ensuring comprehensive global coverage.

The GitHub Actions workflow regularly verifies feed availability, categorizing feeds as:

- ✅ **Active**: Feeds accessible and correctly formatted.
- ❌ **Inactive**: Feeds broken or malformed.
- ⚠️ **Transient Issues**: Feeds with temporary access issues.

The validation ensures that the curated list remains current and reliable for monitoring global security events.

## License

This project is released under the MIT License, allowing permissive reuse, modification, and distribution.

**Important Clarification**: This license applies exclusively to the curated list, its organization, and associated scripts created within this project. The individual RSS feed content and their intellectual property remain the property of their respective publishers and are subject to their original terms of use.