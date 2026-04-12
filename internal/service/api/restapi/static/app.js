(function () {
  var pathParts = window.location.pathname.split('/').filter(Boolean);

  // Detect /confirm/{token} path.
  if (pathParts[0] === 'confirm' && pathParts[1]) {
    showConfirmSection(pathParts[1]);
    return;
  }

  // Detect /unsubscribe/{token} path.
  if (pathParts[0] === 'unsubscribe' && pathParts[1]) {
    showUnsubscribeSection(pathParts[1]);
    return;
  }

  var emailInput  = document.getElementById('email');
  var repoInput   = document.getElementById('repo');
  var submitBtn   = document.getElementById('submitBtn');
  var emailError  = document.getElementById('emailError');
  var repoError   = document.getElementById('repoError');

  // Mirror backend validation rules.
  var repoPattern = /^[A-Za-z0-9_.\-]+\/[A-Za-z0-9_.\-]+$/;

  function isValidEmail(v) {
    // Matches net/mail.ParseAddress: must contain @ with non-empty local and domain parts.
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);
  }

  function isValidRepo(v) {
    return repoPattern.test(v);
  }

  function setFieldError(input, errorEl, msg) {
    errorEl.textContent = msg;
    input.classList.toggle('input-error', !!msg);
  }

  function validateFields() {
    var email = emailInput.value.trim();
    var repo  = repoInput.value.trim();
    var emailMsg = '';
    var repoMsg  = '';

    if (email && !isValidEmail(email)) {
      emailMsg = 'Enter a valid email address.';
    }
    if (repo && !isValidRepo(repo)) {
      repoMsg = 'Use owner/repo format, e.g. golang/go.';
    }

    setFieldError(emailInput, emailError, emailMsg);
    setFieldError(repoInput,  repoError,  repoMsg);

    submitBtn.disabled = !email || !repo || !!emailMsg || !!repoMsg;
  }

  function updateSubmitState() {
    validateFields();
  }

  emailInput.addEventListener('input', updateSubmitState);
  repoInput.addEventListener('input', updateSubmitState);

  document.getElementById('subscribeForm').addEventListener('submit', function (e) {
    e.preventDefault();
    handleSubscribe();
  });

  function handleSubscribe() {
    var btn     = document.getElementById('submitBtn');
    var alertEl = document.getElementById('subscribeAlert');

    hideAlert(alertEl);
    setFieldError(emailInput, emailError, '');
    setFieldError(repoInput,  repoError,  '');
    validateFields();
    if (btn.disabled) return;

    var email = emailInput.value.trim();
    var repo  = repoInput.value.trim();

    btn.disabled = true;
    btn.textContent = 'Subscribing…';

    var body = new URLSearchParams({ email: email, repo: repo });

    fetch('/api/subscribe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body.toString()
    })
    .then(function (res) {
      if (res.ok) {
        showAlert(alertEl, 'success', '✓ Almost there! Check your inbox for a confirmation email.');
        document.getElementById('subscribeForm').reset();
        setFieldError(emailInput, emailError, '');
        setFieldError(repoInput,  repoError,  '');
        updateSubmitState();
        return;
      }
      var fallback = subscribeErrorMessage(res.status, email, repo);
      return errorMessage(res, fallback).then(function (msg) {
        if (res.status === 400) {
          // Show the error under the most likely offending field.
          if (!isValidEmail(email)) {
            setFieldError(emailInput, emailError, msg);
          } else {
            setFieldError(repoInput, repoError, msg);
          }
        } else if (res.status === 409) {
          showAlert(alertEl, 'error', msg);
          setFieldError(repoInput, repoError, msg);
        } else {
          showAlert(alertEl, 'error', msg);
        }
      });
    })
    .catch(function () {
      showAlert(alertEl, 'error', 'Network error. Please check your connection and try again.');
    })
    .finally(function () {
      btn.textContent = 'Subscribe';
      updateSubmitState();
    });
  }

  function subscribeErrorMessage(status, email, repo) {
    if (status === 409) return 'The email ' + email + ' is already subscribed to ' + repo;
    if (status === 404) return 'Repository not found on GitHub. Check the owner/repo name.';
    if (status === 400) return 'Invalid email or repository format.';
    return 'Something went wrong (HTTP ' + status + '). Please try again later.';
  }

  function showConfirmSection(token) {
    document.getElementById('subscribeSection').style.display = 'none';
    document.getElementById('confirmSection').style.display   = 'block';

    fetch('/api/confirm/' + encodeURIComponent(token))
      .then(function (res) {
        var icon    = document.getElementById('confirmIcon');
        var alertEl = document.getElementById('confirmAlert');
        if (res.ok) {
          icon.textContent = '✅';
          showAlert(alertEl, 'success', '✓ Your subscription is confirmed! You will receive release notifications by email.');
          return;
        }
        icon.textContent = '❌';
        var fallback = confirmErrorMessage(res.status);
        return errorMessage(res, fallback).then(function (msg) {
          showAlert(alertEl, 'error', msg);
        });
      })
      .catch(function () {
        document.getElementById('confirmIcon').textContent = '❌';
        showAlert(document.getElementById('confirmAlert'), 'error', 'Network error. Please check your connection and try again.');
      });
  }

  function confirmErrorMessage(status) {
    if (status === 404) return 'Confirmation token not found or already used.';
    if (status === 400) return 'Invalid confirmation token.';
    return 'Something went wrong (HTTP ' + status + '). Please try again later.';
  }

  function showUnsubscribeSection(token) {
    document.getElementById('subscribeSection').style.display = 'none';
    document.getElementById('confirmSection').style.display   = 'none';
    document.getElementById('unsubscribeSection').style.display = 'block';

    fetch('/api/unsubscribe/' + encodeURIComponent(token))
      .then(function (res) {
        var icon    = document.getElementById('unsubscribeIcon');
        var alertEl = document.getElementById('unsubscribeAlert');
        if (res.ok) {
          icon.textContent = '📭';
          showAlert(alertEl, 'success', 'You have been unsubscribed from release notifications.');
          return;
        }

        icon.textContent = '❌';
        var fallback = unsubscribeErrorMessage(res.status);
        return errorMessage(res, fallback).then(function (msg) {
          showAlert(alertEl, 'error', msg);
        });
      })
      .catch(function () {
        document.getElementById('unsubscribeIcon').textContent = '❌';
        showAlert(document.getElementById('unsubscribeAlert'), 'error', 'Network error. Please check your connection and try again.');
      });
  }

  function unsubscribeErrorMessage(status) {
    if (status === 404) return 'Unsubscribe token not found.';
    if (status === 400) return 'Invalid unsubscribe token.';
    return 'Something went wrong (HTTP ' + status + '). Please try again later.';
  }

  /**
   * Try to read a JSON body with a "message" field from the response.
   * Falls back to the provided fallback string if the body is absent or not JSON.
   */
  function errorMessage(res, fallback) {
    return res.text().then(function (text) {
      if (!text) return fallback;
      try {
        var data = JSON.parse(text);
        if (data && typeof data.message === 'string' && data.message) {
          return data.message;
        }
      } catch (_) { /* not JSON */ }
      return fallback;
    }).catch(function () {
      return fallback;
    });
  }

  function showAlert(el, type, msg) {
    el.className = 'alert ' + type;
    el.textContent = msg;
    el.style.display = 'block';
  }

  function hideAlert(el) {
    el.className = 'alert';
    el.style.display = 'none';
    el.textContent = '';
  }
}());
