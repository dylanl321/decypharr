const fs = require('fs');
const path = require('path');

const statsHtml = fs.readFileSync(
    path.join(__dirname, '../pkg/server/templates/stats.html'),
    'utf8'
);
const start = statsHtml.indexOf('<script>') + '<script>'.length;
const end = statsHtml.lastIndexOf('</script>');
let body = statsHtml.slice(start, end).trim();
body = body.replace(/^document\.addEventListener\('DOMContentLoaded',\s*function\s*\(\)\s*\{/, '');
body = body.replace(/\}\);\s*$/, '');

const out = `// System statistics page (extracted from stats.html)
class StatsPage {
    init() {
${body.split('\n').map((line) => '        ' + line).join('\n')}
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.statsPage = new StatsPage();
    window.statsPage.init();
});
`;

fs.writeFileSync(path.join(__dirname, '../pkg/server/assets/js/stats.js'), out);
