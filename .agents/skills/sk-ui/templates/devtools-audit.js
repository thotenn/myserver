// Paste into the browser console on the running dashboard, at each width you
// care about (320, 390, 768, 1024, 1920). No tooling, no dependencies.
//
// It answers the two questions a screenshot cannot:
//   1. does the document scroll sideways, and WHICH element causes it?
//   2. is any text ellipsised — i.e. did the layout "fit" by hiding content?
//
// Expected at every width: overflow 0, clipped 0.
//
// Why it matters: when the page overflows horizontally, a full-page screenshot
// captures the DOCUMENT width while `width: 100%` elements still resolve
// against the viewport. They then look narrow inside the image, and the bug
// reads as "the cards don't fill the screen" — nowhere near the real cause.

(function () {
  var doc = document.documentElement;
  var viewport = doc.clientWidth;
  var overflow = doc.scrollWidth - viewport;

  // Only the DEEPEST offenders matter: an ancestor is merely carrying its
  // child's width, and reporting it sends you to the wrong element.
  var over = [].slice.call(document.querySelectorAll('*')).filter(function (el) {
    var r = el.getBoundingClientRect();
    return r.right > viewport + 1 || r.width > viewport + 1;
  });
  var culprits = over.filter(function (el) {
    return !over.some(function (other) { return other !== el && el.contains(other); });
  });

  // Ellipsised text: the content is wider than the box it renders in.
  var clipped = [].slice.call(
    document.querySelectorAll('.service-card span, .service-card p, .service-card a')
  ).filter(function (el) {
    return el.scrollWidth > el.clientWidth + 1;
  });

  var grid = document.querySelector('.service-grid');
  var card = grid && grid.firstElementChild;

  console.log('viewport   ', viewport + 'px');
  console.log('overflow   ', overflow > 1 ? '❌ +' + overflow + 'px' : '✅ none');
  console.log('clipped    ', clipped.length ? '❌ ' + clipped.length : '✅ none');
  if (grid) {
    console.log('grid cols  ', getComputedStyle(grid).gridTemplateColumns);
    console.log('card width ', card ? Math.round(card.getBoundingClientRect().width) + 'px' : '—');
  }

  if (culprits.length) {
    console.group('overflowing elements');
    culprits.forEach(function (el) {
      console.log(
        el.tagName.toLowerCase() + '.' + (el.getAttribute('class') || '').slice(0, 60),
        Math.round(el.getBoundingClientRect().width) + 'px',
        '::', (el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 40),
        el
      );
    });
    console.groupEnd();
  }

  if (clipped.length) {
    console.group('ellipsised text');
    clipped.forEach(function (el) {
      console.log((el.textContent || '').trim().slice(0, 50), el);
    });
    console.groupEnd();
  }

  return { viewport: viewport, overflow: overflow, clipped: clipped.length };
})();
