$(function() {
  $('[title]').tooltip();

  var editor = ace.edit("editor", {
    mode: "ace/mode/json",
    selectionStyle: "text",
    tabSize: 2,
    useSoftTabs: true
  });

  editor.setTheme("ace/theme/monokai");
  $.get("api/getConfig", function(data) {
    editor.setValue(data);
    editor.clearSelection();
  });

  $("#generateConfigForm").submit(function(event) {
    var input1 = $("#input1").val();
    var input2 = $("#input2").val();
    var input3 = $("#input3").val();
    var input4 = $("#input4").val();
    var input5 = $("#input5").val();
    var result = `{
  "inbounds": [{
    "port": ` + input1 + `,
    "listen": "127.0.0.1",
    "protocol": "http",
    "settings": {}
  }, {
    "port": ` + input2 + `,
    "listen": "127.0.0.1",
    "protocol": "socks",
    "settings": {
      "udp": true
    }
  }],
  "outbounds": [{
    "protocol": "vmess",
    "settings": {
      "vnext": [{
        "address": "` + input3 + `",
        "port": ` + input4 + `,
        "users": [{ "id": "` + input5 + `" }]
      }]
    }`;
  if ($('#input6').is(":checked")) {
    result += `,
    "mux": { "enabled": true }`;
  }
  result += `
  }`;
  if ($('#input7').is(":checked")) {
    result += `,{
    "protocol": "freedom",
    "tag": "direct",
    "settings": {}
  }],
  "routing": {
    "domainStrategy": "IPOnDemand",
    "rules": [{
      "type": "field",
      "ip": ["geoip:private", "geoip:cn"],
      "domain": [ "geosite:cn" ],
      "outboundTag": "direct"
    }]
  }`;
    } else {
      result += `]`;
    }
    result += `
}`;
    if (confirm("警告：生成客户端配置将会覆盖当前配置，确定继续吗？")) {
      editor.setValue(result);
      editor.clearSelection();
      $("#saveConfig").click();
    }
    event.preventDefault();
  });

  $("#saveConfig").click(function() {
    var error = false;
    editor.getSession().getAnnotations().map(function (value) {
      if (value.type == 'error') {
        alert("保存失败：json配置文件语法格式错误，请修改后再重新保存\n"
        + "第" + (value.row + 1) + "行，第" + (value.column + 1) + "列：" + value.text);
        error = true;
      }
    });
    if (error) {
      return;
    }
    $.post("api/saveConfig", editor.getValue(), function(data) {
      alert("保存成功!");
      manuallyStop = false;
      $.get("api/restart", function(data) {
        updateStatus();
    });
    }).fail(function(data) {
      alert("错误：" + data.responseText);
    });
  });

  $("#startBtn").click(function() {
    $.get("api/start", function(data) {
        alert("启动 V2Ray 成功！");
        $('#v2rayStatus').html('V2Ray 正在运行').attr("class", "badge badge-success");
        manuallyStop = false;
    }).fail(function(data) {
      if (data.responseText === undefined) {
        alert("V2Ray Web UI 已经停止");
      } else {
        alert(data.responseText);
      }
    });
  });

  var manuallyStop = false;

  $("#stopBtn").click(function() {
    $.get("api/stop", function(data) {
      alert("停止 V2Ray 成功！");
      $('#v2rayStatus').html('V2Ray 已经停止').attr("class", "badge badge-danger");
      manuallyStop = true;
    }).fail(function(data) {
      if (data.responseText === undefined) {
        alert("V2Ray Web UI 已经停止");
      } else {
        alert(data.responseText);
      }
    });
  });

  $("#logBtn").click(function() {
    $.get("api/getLog", function(data) {
      $('#logModal .modal-body').html('<pre><code>' + data + '</code></pre>');
      $('#logModal').modal('show')
    }).fail(function(data) {
      if (data.responseText === undefined) {
        alert("V2Ray Web UI 已经停止");
      }
    });
  });

  function updateStatus() {
    if (manuallyStop) {
      return;
    }
    $.get("api/getStatus", function(data) {
      if (data == 'running') {
        $('#v2rayStatus').html('V2Ray 正在运行').attr("class", "badge badge-success");
      } else if (data == 'exit') {
        $('#v2rayStatus').html('V2Ray 已经停止，请查看运行日志').attr("class", "badge badge-danger");
      }
    });
  }

  updateStatus();
  window.setInterval(updateStatus, 3000);
});