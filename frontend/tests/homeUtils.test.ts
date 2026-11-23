import assert from 'node:assert/strict';
import test from 'node:test';
import { defaultExclude, formatExcludePreview } from '../src/pages/Home/homeUtils.ts';

test('defaultExclude escapes #recycle/ so gitignore parsers treat it as a rule instead of a comment', () => {
  const lines = defaultExclude.split(/\r?\n/).map((line) => line.trim());
  assert.equal(lines.includes('\\#recycle/'), true);
  assert.equal(lines.includes('#recycle/'), false);
});

test('formatExcludePreview unescapes leading # and ! rules and skips comments', () => {
  assert.equal(
    formatExcludePreview('# NAS recycle\n\\#recycle/\n\\!keep.txt\n@eaDir/'),
    '#recycle/、!keep.txt、@eaDir/',
  );
  assert.equal(formatExcludePreview('  # only comment\r\n\r\n  # another '), '仅注释');
  assert.equal(formatExcludePreview(''), '仅注释');
  assert.equal(formatExcludePreview(null), '仅注释');
});

test('formatExcludePreview summarizes long rule lists including defaultExclude', () => {
  const preview = formatExcludePreview('*.tmp\r\n\\#recycle/\r\n.git/\r\n@eaDir/\r\n');
  assert.equal(preview, '*.tmp、#recycle/、.git/ 等 4 条');

  const defaultPreview = formatExcludePreview(defaultExclude);
  assert.match(defaultPreview, /^\.DS_Store、\._\*、\.Spotlight-V100\/ 等 \d+ 条$/);
});
