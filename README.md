# vwidentity

Little go code to get a token for apis used by the [myvolkswagen](https://www.volkswagen.de/de/besitzer-und-nutzer/myvolkswagen.html) page. This is my first go code, so do not judge me to harsh.

To test create a `vw.yaml` with:
```yaml
mail: <mail>
password: <password>
```

I run this on a raspbery pi with a working root mail, with ssmtp and a script in the `cron.hourly`:

```bash
#!/bin/bash

DIR="<path to vw.yaml and space to store last run result>"

OLDRESULT=$(cat $DIR/oldresult)
RESULT=$(docker run -v $DIR/vw.yaml:/vwidentity/vw.yaml -it vwidentity | cut -c 21-)
if [ "$OLDRESULT" != "$RESULT" ]; then
    echo "$RESULT" > $DIR/oldresult
    echo "changed"
    echo "new"
    echo "$RESULT"
    echo "old"
    echo "$OLDRESULT"
fi
```

This informs me on every change in the response.