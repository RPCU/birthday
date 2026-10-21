{
  pkgs,
  ...
}:
{
  packages = with pkgs; [
    go
    gofumpt
    golines
    air
  ];

  env.BIRTHDAY_NAME = "Chihiro";
  env.BIRTHDAY_DATE = "07-20";
  env.BIRTHDAY_TIMEZONE = "Europe/Paris";

  enterShell = ''
    echo ""
    echo "Birthday development environment loaded"
    echo ""
    echo "Run with:"
    echo "  go run . serve    # http://127.0.0.1:8080"
    echo ""
  '';
}
